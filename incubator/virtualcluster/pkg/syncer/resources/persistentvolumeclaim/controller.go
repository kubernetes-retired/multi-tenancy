/*
Copyright 2019 The Kubernetes Authors.

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

package persistentvolumeclaim

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

type controller struct {
	config *config.SyncerConfiguration
	// super master pvc client
	pvcClient v1core.PersistentVolumeClaimsGetter
	// super master pvc lister
	pvcLister listersv1.PersistentVolumeClaimLister
	pvcSynced cache.InformerSynced
	// Connect to all tenant master pvc informers
	multiClusterPersistentVolumeClaimController *mc.MultiClusterController
	// Periodic checker
	persistentVolumeClaimPatroller *pa.Patroller
}

func NewPVCController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		config:    config,
		pvcClient: client.CoreV1(),
	}

	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	mcOptions.MaxConcurrentReconciles = constants.DwsControllerWorkerLow
	multiClusterPersistentVolumeClaimController, err := mc.NewMCController("tenant-masters-pvc-controller", &v1.PersistentVolumeClaim{}, *mcOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create persistentVolumeClaim mc controller: %v", err)
	}
	c.multiClusterPersistentVolumeClaimController = multiClusterPersistentVolumeClaimController
	c.pvcLister = informer.Core().V1().PersistentVolumeClaims().Lister()
	if options != nil && options.IsFake {
		c.pvcSynced = func() bool { return true }
	} else {
		c.pvcSynced = informer.Core().V1().PersistentVolumeClaims().Informer().HasSynced
	}

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	persistentVolumeClaimPatroller, err := pa.NewPatroller("pvc-patroller", &v1.PersistentVolumeClaim{}, *patrolOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create persistentVolumeClaim patroller: %v", err)
	}
	c.persistentVolumeClaimPatroller = persistentVolumeClaimPatroller

	return c, multiClusterPersistentVolumeClaimController, nil, nil
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) BackPopulate(string) error {
	return nil
}

func (c *controller) GetListener() listener.ClusterChangeListener {
	return listener.NewMCControllerListener(c.multiClusterPersistentVolumeClaimController)
}
