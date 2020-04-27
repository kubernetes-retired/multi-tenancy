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

package event

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
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
	// super master event client (not used for now)
	client v1core.EventsGetter
	// super master event informer/lister/synced functions
	informer    coreinformers.Interface
	eventLister listersv1.EventLister
	eventSynced cache.InformerSynced
	nsLister    listersv1.NamespaceLister
	nsSynced    cache.InformerSynced

	// Connect to all tenant master event informers
	multiClusterEventController *mc.MultiClusterController
	// UWcontroller
	upwardEventController *uw.UpwardController
}

func Register(
	config *config.SyncerConfiguration,
	client v1core.EventsGetter,
	informer coreinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		client:   client,
		informer: informer,
	}

	// Create the multi cluster pod controller
	options := mc.Options{Reconciler: c}
	multiClusterEventController, err := mc.NewMCController("tenant-masters-event-controller", &v1.Event{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster event controller %v", err)
		return
	}
	c.multiClusterEventController = multiClusterEventController

	c.nsLister = informer.Namespaces().Lister()
	c.nsSynced = informer.Namespaces().Informer().HasSynced

	c.eventLister = informer.Events().Lister()
	c.eventSynced = informer.Events().Informer().HasSynced

	uwOptions := &uw.Options{Reconciler: c}
	uwOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerLow
	upwardEventController, err := uw.NewUWController("event-upward-controller", &v1.Event{}, *uwOptions)
	if err != nil {
		klog.Errorf("failed to create event upward controller %v", err)
		return
	}
	c.upwardEventController = upwardEventController

	informer.Events().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Event:
					return assignPodEvent(t)
				case cache.DeletedFinalStateUnknown:
					if e, ok := t.Obj.(*v1.Event); ok {
						return assignPodEvent(e)
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %v to *v1.Event", obj))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in super master event controller: %v", obj))
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: c.enqueueEvent,
			},
		})

	controllerManager.AddResourceSyncer(c)
}

func assignPodEvent(e *v1.Event) bool {
	return e.InvolvedObject.Kind == "Pod"
}

func (c *controller) enqueueEvent(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %v: %v", obj, err))
		return
	}
	c.upwardEventController.AddToQueue(key)
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) PatrollerDo() {
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	return reconciler.Result{}, nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-event-controller watch cluster %s for event resource", cluster.GetClusterName())
	err := c.multiClusterEventController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s event event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-event-controller stop watching cluster %s for event resource", cluster.GetClusterName())
	c.multiClusterEventController.TeardownClusterResource(cluster)
}
