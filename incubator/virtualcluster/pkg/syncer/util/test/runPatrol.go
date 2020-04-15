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

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	pa "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
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

func RunPatrol(
	newControllerFunc controllerNew,
	testTenant *v1alpha1.Virtualcluster,
	existingObjectInSuper []runtime.Object,
	existingObjectInTenant []runtime.Object,
	waitDWS bool,
	waitUWS bool,
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

	rsOptions := &manager.ResourceSyncerOptions{
		MCOptions:     &mc.Options{Reconciler: fakeDWRc},
		UWOptions:     &uw.Options{Reconciler: fakeUWRc},
		PatrolOptions: &pa.Options{Reconciler: fakePatrolRc},
		IsFake:        true,
	}

	resourceSyncer, _, _, err := newControllerFunc(
		&config.SyncerConfiguration{
			DisableServiceAccountToken: true,
		},
		superClient.CoreV1(),
		superInformer.Core().V1(),
		rsOptions,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating controller: %v", err)
	}
	fakePatrolRc.SetResourceSyncer(resourceSyncer)
	fakeDWRc.SetResourceSyncer(resourceSyncer)
	fakeUWRc.SetResourceSyncer(resourceSyncer)

	// register tenant cluster to controller.
	resourceSyncer.AddCluster(tenantCluster)

	stopCh := make(chan struct{})
	defer close(stopCh)

	// add object to super informer.
	for _, each := range existingObjectInSuper {
		informer := getObjectInformer(superInformer.Core().V1(), each)
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

	return tenantClientset.Actions(), superClient.Actions(), nil
}
