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

	v1 "k8s.io/api/core/v1"
	storageV1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	fakeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	fakevcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned/fake"
	vcinformerFactory "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	utilscheme "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/scheme"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/cluster"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

type fakeReconciler struct {
	resourceSyncer manager.ResourceSyncer
	errCh          chan error
}

func (r *fakeReconciler) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	var res reconciler.Result
	var err error
	if r.resourceSyncer != nil {
		res, err = r.resourceSyncer.Reconcile(request)
	} else {
		res, err = reconciler.Result{}, fmt.Errorf("fake reconciler's controller is not initialized")
	}
	select {
	case <-r.errCh:
	default:
		// if channel not closed
		r.errCh <- err
	}

	// Make sure Reconcile is called once by returning no error.
	return res, nil
}

func (r *fakeReconciler) SetResourceSyncer(c manager.ResourceSyncer) {
	r.resourceSyncer = c
}

type FakeClientSetMutator func(tenantClientset, superClientset *fake.Clientset)

func RunDownwardSync(
	newControllerFunc manager.ResourceSyncerNew,
	testTenant *v1alpha1.VirtualCluster,
	existingObjectInSuper []runtime.Object,
	existingObjectInTenant []runtime.Object,
	enqueueObject runtime.Object,
	clientSetMutator FakeClientSetMutator,
) (actions []core.Action, reconcileError error, err error) {
	registerDefaultScheme()
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
	vcInformer := vcinformerFactory.NewSharedInformerFactory(vcClient, 0).Tenancy().V1alpha1().VirtualClusters()

	if clientSetMutator != nil {
		clientSetMutator(tenantClientset, superClient)
	}

	// setup fake controller
	syncErr := make(chan error)
	defer close(syncErr)
	fakeRc := &fakeReconciler{errCh: syncErr}
	rsOptions := manager.ResourceSyncerOptions{
		MCOptions: &mc.Options{Reconciler: fakeRc},
		IsFake:    true,
	}

	resourceSyncer, err := newControllerFunc(
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
		return nil, nil, fmt.Errorf("error creating dws controller: %v", err)
	}
	fakeRc.SetResourceSyncer(resourceSyncer)

	// register tenant cluster to controller.
	resourceSyncer.GetListener().AddCluster(tenantCluster)
	resourceSyncer.GetListener().WatchCluster(tenantCluster)
	defer resourceSyncer.GetListener().RemoveCluster(tenantCluster)

	stopCh := make(chan struct{})
	defer close(stopCh)
	go resourceSyncer.StartDWS(stopCh)

	// add object to super informer.
	for _, each := range existingObjectInSuper {
		informer := superInformer.InformerFor(each, nil)
		informer.GetStore().Add(each)
	}

	// start testing
	if err := resourceSyncer.GetMCController().RequeueObject(conversion.ToClusterKey(testTenant), enqueueObject); err != nil {
		return nil, nil, fmt.Errorf("error enqueue object %v: %v", enqueueObject, err)
	}

	// wait to be called
	select {
	case reconcileError = <-syncErr:
	case <-time.After(10 * time.Second):
		return nil, nil, fmt.Errorf("timeout wating for sync")
	}

	return superClient.Actions(), reconcileError, nil
}

func registerDefaultScheme() {
	utilscheme.Scheme.AddKnownTypePair(&v1.Namespace{}, &v1.NamespaceList{},
		&v1.Service{}, &v1.ServiceList{},
		&v1.Pod{}, &v1.PodList{},
		&v1.ServiceAccount{}, &v1.ServiceAccountList{},
		&v1.Secret{}, &v1.SecretList{},
		&v1.Node{}, &v1.NodeList{},
		&v1.PersistentVolume{}, &v1.PersistentVolumeList{},
		&v1.PersistentVolumeClaim{}, &v1.PersistentVolumeClaimList{},
		&v1.ConfigMap{}, &v1.ConfigMapList{},
		&v1.Endpoints{}, &v1.EndpointsList{},
		&v1.Event{}, &v1.EventList{},
		&storageV1.StorageClass{}, &storageV1.StorageClassList{})
}
