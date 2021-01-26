/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	fakeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	fakevcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned/fake"
	vcinformerFactory "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/cluster"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

type fakePatrolReconciler struct {
	resourceSyncer manager.ResourceSyncer
	errCh          chan error
}

func (r *fakePatrolReconciler) PatrollerDo() {
	var err error
	if r.resourceSyncer != nil {
		r.resourceSyncer.PatrollerDo()
		err = nil
	} else {
		err = fmt.Errorf("fake patrol reconciler is not initialized")
	}
	r.errCh <- err
}

func (r *fakePatrolReconciler) SetResourceSyncer(c manager.ResourceSyncer) {
	r.resourceSyncer = c
}

type controllerStateModifier func(manager.ResourceSyncer)

func RunPatrol(
	newControllerFunc manager.ResourceSyncerNew,
	testTenant *v1alpha1.VirtualCluster,
	existingObjectInSuper []runtime.Object,
	existingObjectInTenant []runtime.Object,
	existingObjectInVCClient []runtime.Object,
	waitDWS bool,
	waitUWS bool,
	controllerStateModifyFunc controllerStateModifier,
) ([]core.Action, []core.Action, error) {
	// setup fake tenant cluster
	tenantClientset := fake.NewSimpleClientset()
	tenantClient := fakeClient.NewFakeClient()
	if existingObjectInTenant != nil {
		tenantClientset = fake.NewSimpleClientset(existingObjectInTenant...)
		// For controller runtime client, if the informer cache is empty, the request goes to client obj tracker.
		// Hence we don't have to populate the infomer cache.
		tenantClient = fakeClient.NewFakeClient(existingObjectInTenant...)
	}
	tenantCluster, err := cluster.NewFakeTenantCluster(testTenant, tenantClientset, tenantClient)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating tenantCluster: %v", err)
	}

	// setup fake super cluster
	superClient := fake.NewSimpleClientset()
	if existingObjectInSuper != nil {
		superClient = fake.NewSimpleClientset(existingObjectInSuper...)
	}
	superInformer := informers.NewSharedInformerFactory(superClient, 0)
	// setup fake vc client
	vcClient := fakevcclient.NewSimpleClientset()
	if existingObjectInVCClient != nil {
		vcClient = fakevcclient.NewSimpleClientset(existingObjectInVCClient...)
	}
	vcInformer := vcinformerFactory.NewSharedInformerFactory(vcClient, 0).Tenancy().V1alpha1().VirtualClusters()
	// Add obj to vc informer cache
	for _, each := range existingObjectInVCClient {
		_, ok := each.(*v1alpha1.VirtualCluster)
		if !ok {
			return nil, nil, fmt.Errorf("only vc object can be added to vc informer cache: %v", each)
		}
		vcInformer.Informer().GetStore().Add(each)
	}

	// setup fake controller
	syncDWS := make(chan error)
	defer close(syncDWS)
	syncUWS := make(chan error)
	defer close(syncUWS)
	syncPatrol := make(chan error)
	defer close(syncPatrol)

	fakePatrolRc := &fakePatrolReconciler{errCh: syncPatrol}
	fakeDWRc := &fakeReconciler{errCh: syncDWS}
	fakeUWRc := &fakeUWReconciler{errCh: syncUWS}

	rsOptions := manager.ResourceSyncerOptions{
		MCOptions:     &mc.Options{Reconciler: fakeDWRc},
		UWOptions:     &uw.Options{Reconciler: fakeUWRc},
		PatrolOptions: &pa.Options{Reconciler: fakePatrolRc},
		IsFake:        true,
	}

	resourceSyncer, _, _, err := newControllerFunc(
		&config.SyncerConfiguration{
			DisableServiceAccountToken: true,
		},
		superClient,
		superInformer,
		vcClient,
		vcInformer,
		rsOptions,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating controller: %v", err)
	}
	fakePatrolRc.SetResourceSyncer(resourceSyncer)
	fakeDWRc.SetResourceSyncer(resourceSyncer)
	fakeUWRc.SetResourceSyncer(resourceSyncer)

	// Update controller internal state
	if controllerStateModifyFunc != nil {
		controllerStateModifyFunc(resourceSyncer)
	}

	// register tenant cluster to controller.
	resourceSyncer.GetListener().AddCluster(tenantCluster)
	resourceSyncer.GetListener().WatchCluster(tenantCluster)
	defer resourceSyncer.GetListener().RemoveCluster(tenantCluster)

	stopCh := make(chan struct{})
	defer close(stopCh)

	// add object to super informer.
	for _, each := range existingObjectInSuper {
		informer := getObjectInformer(superInformer, each)
		informer.GetStore().Add(each)
	}
	go resourceSyncer.StartDWS(stopCh)
	go resourceSyncer.StartUWS(stopCh)
	go resourceSyncer.StartPatrol(stopCh)

	// wait to be called
	select {
	case _ = <-syncPatrol:
	case <-time.After(10 * time.Second):
		return nil, nil, fmt.Errorf("timeout waiting for syncPatrol")
	}
	if waitDWS {
		select {
		case _ = <-syncDWS:
		case <-time.After(10 * time.Second):
			return nil, nil, fmt.Errorf("timeout waiting for syncDWS")
		}
	}
	if waitUWS {
		select {
		case _ = <-syncUWS:
		case <-time.After(10 * time.Second):
			return nil, nil, fmt.Errorf("timeout waiting for syncUWS")
		}
	}

	tenantActions := tenantClientset.Actions()
	superActions := superClient.Actions()

	// filter readonly action while we check the write operation in tests.
	for i := 0; i < len(tenantActions); {
		if tenantActions[i].GetVerb() == "get" {
			tenantActions = append(tenantActions[:i], tenantActions[i+1:]...)
		} else {
			i++
		}
	}
	for i := 0; i < len(superActions); {
		if superActions[i].GetVerb() == "get" {
			superActions = append(superActions[:i], superActions[i+1:]...)
		} else {
			i++
		}
	}

	return tenantActions, superActions, nil
}
