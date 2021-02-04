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
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

type controller struct {
	manager.BaseResourceSyncer
	// super master client
	client v1core.CoreV1Interface
	// super master pv/pvc lister/synced functions
	informer  coreinformers.Interface
	pvLister  listersv1.PersistentVolumeLister
	pvSynced  cache.InformerSynced
	pvcLister listersv1.PersistentVolumeClaimLister
	pvcSynced cache.InformerSynced
}

func NewPVController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options manager.ResourceSyncerOptions) (manager.ResourceSyncer, error) {
	c := &controller{
		BaseResourceSyncer: manager.BaseResourceSyncer{
			Config: config,
		},
		client:   client.CoreV1(),
		informer: informer.Core().V1(),
	}

	var err error
	c.MultiClusterController, err = mc.NewMCController(&v1.PersistentVolume{}, &v1.PersistentVolumeList{}, c, mc.WithOptions(options.MCOptions))
	if err != nil {
		return nil, err
	}

	c.pvLister = c.informer.PersistentVolumes().Lister()
	c.pvcLister = c.informer.PersistentVolumeClaims().Lister()

	if options.IsFake {
		c.pvSynced = func() bool { return true }
		c.pvcSynced = func() bool { return true }
	} else {
		c.pvSynced = c.informer.PersistentVolumes().Informer().HasSynced
		c.pvcSynced = c.informer.PersistentVolumeClaims().Informer().HasSynced
	}

	c.UpwardController, err = uw.NewUWController(&v1.PersistentVolume{}, c, uw.WithOptions(options.UWOptions))
	if err != nil {
		return nil, err
	}

	c.Patroller, err = pa.NewPatroller(&v1.PersistentVolume{}, c, pa.WithOptions(options.PatrolOptions))
	if err != nil {
		return nil, err
	}

	c.informer.PersistentVolumes().Informer().AddEventHandler(
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

	return c, nil
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
	c.UpwardController.AddToQueue(key)
}
