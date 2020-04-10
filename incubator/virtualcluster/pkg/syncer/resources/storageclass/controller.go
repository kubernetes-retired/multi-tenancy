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

package storageclass

import (
	"fmt"
	"time"

	v1 "k8s.io/api/storage/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	storageinformers "k8s.io/client-go/informers/storage/v1"
	v1storage "k8s.io/client-go/kubernetes/typed/storage/v1"
	listersv1 "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	uw "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
)

type controller struct {
	// super master storageclasses client
	client v1storage.StorageClassesGetter
	// super master storageclasses informer/lister/synced functions
	informer           storageinformers.Interface
	storageclassLister listersv1.StorageClassLister
	storageclassSynced cache.InformerSynced

	// Connect to all tenant master storageclass informers
	multiClusterStorageClassController *mc.MultiClusterController
	// UWcontroller
	upwardStorageClassController *uw.UpwardController

	// Checker timer
	periodCheckerPeriod time.Duration
}

func Register(
	config *config.SyncerConfiguration,
	client v1storage.StorageClassesGetter,
	informer storageinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		client:              client,
		informer:            informer,
		periodCheckerPeriod: 60 * time.Second,
	}

	options := mc.Options{Reconciler: c}
	multiClusterStorageClassController, err := mc.NewMCController("tenant-masters-storageclass-controller", &v1.StorageClass{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster event controller %v", err)
		return
	}
	c.multiClusterStorageClassController = multiClusterStorageClassController

	c.storageclassLister = informer.StorageClasses().Lister()
	c.storageclassSynced = informer.StorageClasses().Informer().HasSynced

	uwOptions := &uw.Options{Reconciler: c}
	uwOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerLow
	upwardStorageClassController, err := uw.NewUWController("storageclass-upward-controller", &v1.StorageClass{}, *uwOptions)
	if err != nil {
		klog.Errorf("failed to create storageclass upward controller %v", err)
		return
	}
	c.upwardStorageClassController = upwardStorageClassController

	informer.StorageClasses().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.StorageClass:
					return publicStorageClass(t)
				case cache.DeletedFinalStateUnknown:
					if e, ok := t.Obj.(*v1.StorageClass); ok {
						return publicStorageClass(e)
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %v to *v1.StorageClass", obj))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in super master storageclass controller: %v", obj))
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: c.enqueueStorageClass,
				UpdateFunc: func(oldObj, newObj interface{}) {
					newStorageClass := newObj.(*v1.StorageClass)
					oldStorageClass := oldObj.(*v1.StorageClass)
					if newStorageClass.ResourceVersion != oldStorageClass.ResourceVersion {
						c.enqueueStorageClass(newObj)
					}
				},
				DeleteFunc: c.enqueueStorageClass,
			},
		})

	controllerManager.AddResourceSyncer(c)
}

func publicStorageClass(e *v1.StorageClass) bool {
	// We only backpopulate specific storageclass to tenant masters
	return e.Labels[constants.PublicObjectKey] == "true"
}

func (c *controller) enqueueStorageClass(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %v: %v", obj, err))
		return
	}

	clusterNames := c.multiClusterStorageClassController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("No tenant masters, stop backpopulate storageclass %v", key)
		return
	}

	for _, clusterName := range clusterNames {
		c.upwardStorageClassController.AddToQueue(reconciler.UwsRequest{Key: clusterName + "/" + key})
	}
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	return reconciler.Result{}, nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-storageclass-controller watch cluster %s for storageclass resource", cluster.GetClusterName())
	err := c.multiClusterStorageClassController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s storageclass: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-storageclass-controller stop watching cluster %s for storageclass resource", cluster.GetClusterName())
	c.multiClusterStorageClassController.TeardownClusterResource(cluster)
}
