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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	core "k8s.io/client-go/testing"
	cache "k8s.io/client-go/tools/cache"
	fakeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	uw "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
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
	r.errCh <- err
	return res, err
}

func (r *fakeReconciler) SetResourceSyncer(c manager.ResourceSyncer) {
	r.resourceSyncer = c
}

type controllerNew func(*config.SyncerConfiguration, corev1.CoreV1Interface, coreinformers.Interface, *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error)

func RunDownwardSync(
	newControllerFunc controllerNew,
	testTenant *v1alpha1.Virtualcluster,
	existingObjectInSuper []runtime.Object,
	existingObjectInTenant []runtime.Object,
	enqueueObject runtime.Object,
) (actions []core.Action, reconcileError error, err error) {
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
	syncErr := make(chan error)
	defer close(syncErr)
	fakeRc := &fakeReconciler{errCh: syncErr}
	rsOptions := &manager.ResourceSyncerOptions{
		MCOptions: &mc.Options{Reconciler: fakeRc},
		IsFake:    true,
	}

	resourceSyncer, mccontroller, _, err := newControllerFunc(
		&config.SyncerConfiguration{
			DisableServiceAccountToken: true,
		},
		superClient.CoreV1(),
		superInformer.Core().V1(),
		rsOptions,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating dws controller: %v", err)
	}
	fakeRc.SetResourceSyncer(resourceSyncer)

	// register tenant cluster to controller.
	resourceSyncer.AddCluster(tenantCluster)

	stopCh := make(chan struct{})
	defer close(stopCh)
	go resourceSyncer.StartDWS(stopCh)

	// add object to super informer.
	for _, each := range existingObjectInSuper {
		informer := getObjectInformer(superInformer.Core().V1(), each)
		informer.GetStore().Add(each)
	}

	// start testing
	if err := mccontroller.RequeueObject(conversion.ToClusterKey(testTenant), enqueueObject); err != nil {
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

func getObjectInformer(informer coreinformers.Interface, obj runtime.Object) cache.SharedIndexInformer {
	switch obj.(type) {
	case *v1.Namespace:
		return informer.Namespaces().Informer()
	case *v1.Service:
		return informer.Services().Informer()
	case *v1.Pod:
		return informer.Pods().Informer()
	case *v1.ServiceAccount:
		return informer.ServiceAccounts().Informer()
	case *v1.Secret:
		return informer.Secrets().Informer()
	case *v1.Node:
		return informer.Nodes().Informer()
	default:
		return nil
	}
}
