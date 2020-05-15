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

package persistentvolume

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
)

type controller struct {
	config *config.SyncerConfiguration
	// super master client
	client v1core.CoreV1Interface
	// super master pv/pvc lister/synced functions
	informer  coreinformers.Interface
	pvLister  listersv1.PersistentVolumeLister
	pvSynced  cache.InformerSynced
	pvcLister listersv1.PersistentVolumeClaimLister
	pvcSynced cache.InformerSynced
	// Connect to all tenant master pv informers
	multiClusterPersistentVolumeController *mc.MultiClusterController
	// UWcontroller
	upwardPersistentVolumeController *uw.UpwardController
	// Periodic checker
	persistentVolumePatroller *pa.Patroller
}

func Register(
	config *config.SyncerConfiguration,
	client v1core.CoreV1Interface,
	informer coreinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c, _, _, err := NewPVController(config, client, informer, nil)
	if err != nil {
		klog.Errorf("failed to create multi cluster PV controller %v", err)
		return
	}
	controllerManager.AddResourceSyncer(c)
}

func NewPVController(config *config.SyncerConfiguration,
	client v1core.CoreV1Interface,
	informer coreinformers.Interface,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		config:   config,
		client:   client,
		informer: informer,
	}

	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	multiClusterPersistentVolumeController, err := mc.NewMCController("tenant-masters-pv-controller", &v1.PersistentVolume{}, *mcOptions)
	if err != nil {
		klog.Errorf("failed to create multi cluster PersistentVolume controller %v", err)
		return nil, nil, nil, err
	}
	c.multiClusterPersistentVolumeController = multiClusterPersistentVolumeController
	c.pvLister = informer.PersistentVolumes().Lister()
	c.pvcLister = informer.PersistentVolumeClaims().Lister()

	if options != nil && options.IsFake {
		c.pvSynced = func() bool { return true }
		c.pvcSynced = func() bool { return true }
	} else {
		c.pvSynced = informer.PersistentVolumes().Informer().HasSynced
		c.pvcSynced = informer.PersistentVolumeClaims().Informer().HasSynced
	}

	var uwOptions *uw.Options
	if options == nil || options.UWOptions == nil {
		uwOptions = &uw.Options{Reconciler: c}
	} else {
		uwOptions = options.UWOptions
	}
	uwOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerLow
	upwardPersistentVolumeController, err := uw.NewUWController("pv-upward-controller", &v1.PersistentVolume{}, *uwOptions)
	if err != nil {
		klog.Errorf("failed to create pv upward controller %v", err)
		return nil, nil, nil, err
	}
	c.upwardPersistentVolumeController = upwardPersistentVolumeController

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	persistentVolumePatroller, err := pa.NewPatroller("persistentVolume-patroller", *patrolOptions)
	if err != nil {
		klog.Errorf("failed to create persistentVolume patroller %v", err)
		return nil, nil, nil, err
	}
	c.persistentVolumePatroller = persistentVolumePatroller

	informer.PersistentVolumes().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.PersistentVolume:
					return boundPersistentVolume(t)
				case cache.DeletedFinalStateUnknown:
					if e, ok := t.Obj.(*v1.PersistentVolume); ok {
						return boundPersistentVolume(e)
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %v to *v1.PersistentVolume", obj))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in super master persistentvolume controller: %v", obj))
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: c.enqueuePersistentVolume,
				UpdateFunc: func(oldObj, newObj interface{}) {
					newPV := newObj.(*v1.PersistentVolume)
					oldPV := oldObj.(*v1.PersistentVolume)
					if newPV.ResourceVersion != oldPV.ResourceVersion {
						c.enqueuePersistentVolume(newObj)
					}
				},
				DeleteFunc: c.enqueuePersistentVolume,
			},
		})

	return c, multiClusterPersistentVolumeController, upwardPersistentVolumeController, nil
}

func boundPersistentVolume(e *v1.PersistentVolume) bool {
	return e.Spec.ClaimRef != nil
}

func (c *controller) enqueuePersistentVolume(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %v: %v", obj, err))
		return
	}
	c.upwardPersistentVolumeController.AddToQueue(key)
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	return reconciler.Result{}, nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-pv-controller watch cluster %s for pv resource", cluster.GetClusterName())
	err := c.multiClusterPersistentVolumeController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s pv event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-pv-controller stop watching cluster %s for pv resource", cluster.GetClusterName())
	c.multiClusterPersistentVolumeController.TeardownClusterResource(cluster)
}
