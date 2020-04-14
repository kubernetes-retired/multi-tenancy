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
	v1 "k8s.io/api/core/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	pa "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
)

type controller struct {
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

func Register(
	config *config.SyncerConfiguration,
	pvcClient v1core.PersistentVolumeClaimsGetter,
	pvcInformer coreinformers.PersistentVolumeClaimInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		pvcClient: pvcClient,
	}

	// Create the multi cluster PersistentVolumeClaim controller
	options := mc.Options{Reconciler: c, MaxConcurrentReconciles: constants.DwsControllerWorkerLow}
	multiClusterPersistentVolumeClaimController, err := mc.NewMCController("tenant-masters-pvc-controller", &v1.PersistentVolumeClaim{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster PersistentVolumeClaim controller %v", err)
		return
	}
	c.multiClusterPersistentVolumeClaimController = multiClusterPersistentVolumeClaimController
	c.pvcLister = pvcInformer.Lister()
	c.pvcSynced = pvcInformer.Informer().HasSynced

	patrolOptions := &pa.Options{Reconciler: c}
	persistentVolumeClaimPatroller, err := pa.NewPatroller("pvc-patroller", *patrolOptions)
	if err != nil {
		klog.Errorf("failed to create persistentVolumeClaim patroller %v", err)
		return
	}
	c.persistentVolumeClaimPatroller = persistentVolumeClaimPatroller

	controllerManager.AddResourceSyncer(c)
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) BackPopulate(string) error {
	return nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-pvc-controller watch cluster %s for pvc resource", cluster.GetClusterName())
	err := c.multiClusterPersistentVolumeClaimController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s pvc event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-pvc-controller stop watching cluster %s for pvc resource", cluster.GetClusterName())
	c.multiClusterPersistentVolumeClaimController.TeardownClusterResource(cluster)
}
