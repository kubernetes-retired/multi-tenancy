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

package priorityclass

import (
	"fmt"

	v1 "k8s.io/api/scheduling/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	priorityclassinformers "k8s.io/client-go/informers/scheduling/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1priorityclass "k8s.io/client-go/kubernetes/typed/scheduling/v1"
	listersv1 "k8s.io/client-go/listers/scheduling/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

type controller struct {
	config *config.SyncerConfiguration
	// super master priorityclasses client
	client v1priorityclass.PriorityClassesGetter
	// super master priorityclasses informer/lister/synced functions
	informer            priorityclassinformers.Interface //weiling
	priorityclassLister listersv1.PriorityClassLister
	priorityclassSynced cache.InformerSynced

	// Connect to all tenant master priorityclass informers
	multiClusterPriorityClassController *mc.MultiClusterController
	// UWcontroller
	upwardPriorityClassController *uw.UpwardController
	// Periodic checker
	priorityClassPatroller *pa.Patroller
}

func NewPriorityClassController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		config:   config,
		client:   client.SchedulingV1(),
		informer: informer.Scheduling().V1(),
	}

	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	mcOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerLow
	multiClusterPriorityClassController, err := mc.NewMCController("tenant-masters-priorityclass-controller", &v1.PriorityClass{}, *mcOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create priorityClass mc controller: %v", err)
	}
	c.multiClusterPriorityClassController = multiClusterPriorityClassController

	c.priorityclassLister = informer.Scheduling().V1().PriorityClasses().Lister()
	if options != nil && options.IsFake {
		c.priorityclassSynced = func() bool { return true }
	} else {
		c.priorityclassSynced = informer.Scheduling().V1().PriorityClasses().Informer().HasSynced
	}

	var uwOptions *uw.Options
	if options == nil || options.UWOptions == nil {
		uwOptions = &uw.Options{Reconciler: c}
	} else {
		uwOptions = options.UWOptions
	}
	uwOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerLow
	upwardPriorityClassController, err := uw.NewUWController("priorityclass-upward-controller", &v1.PriorityClass{}, *uwOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create priorityclass upward controller: %v", err)
	}
	c.upwardPriorityClassController = upwardPriorityClassController

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	priorityClassPatroller, err := pa.NewPatroller("priorityClass-patroller", &v1.PriorityClass{}, *patrolOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create priorityClass patroller: %v", err)
	}
	c.priorityClassPatroller = priorityClassPatroller

	c.informer.PriorityClasses().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.PriorityClass:
					return publicPriorityClass(t)
				case cache.DeletedFinalStateUnknown:
					if e, ok := t.Obj.(*v1.PriorityClass); ok {
						return publicPriorityClass(e)
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %v to *v1.PriorityClass", obj))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in super master priorityclass controller: %v", obj))
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: c.enqueuePriorityClass,
				UpdateFunc: func(oldObj, newObj interface{}) {
					newPriorityClass := newObj.(*v1.PriorityClass)
					oldPriorityClass := oldObj.(*v1.PriorityClass)
					if newPriorityClass.ResourceVersion != oldPriorityClass.ResourceVersion {
						c.enqueuePriorityClass(newObj)
					}
				},
				DeleteFunc: c.enqueuePriorityClass,
			},
		})
	return c, multiClusterPriorityClassController, upwardPriorityClassController, nil
}

func publicPriorityClass(e *v1.PriorityClass) bool {
	// We only backpopulate specific priorityclass to tenant masters
	return e.Labels[constants.PublicObjectKey] == "true"
}

func (c *controller) enqueuePriorityClass(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %v: %v", obj, err))
		return
	}

	clusterNames := c.multiClusterPriorityClassController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("No tenant masters, stop backpopulate priorityclass %v", key)
		return
	}

	for _, clusterName := range clusterNames {
		c.upwardPriorityClassController.AddToQueue(clusterName + "/" + key)
	}
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	return reconciler.Result{}, nil
}

func (c *controller) GetListener() listener.ClusterChangeListener {
	return listener.NewMCControllerListener(c.multiClusterPriorityClassController)
}
