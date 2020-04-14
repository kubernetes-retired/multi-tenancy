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

package service

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
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	pa "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
)

type controller struct {
	// super master service client
	serviceClient v1core.ServicesGetter
	// super master informer/listers/synced functions
	serviceLister listersv1.ServiceLister
	serviceSynced cache.InformerSynced
	// Connect to all tenant master service informers
	multiClusterServiceController *mc.MultiClusterController
	// UWcontroller
	upwardServiceController *uw.UpwardController
	// Periodic checker
	servicePatroller *pa.Patroller
}

func Register(
	config *config.SyncerConfiguration,
	serviceClient v1core.CoreV1Interface,
	informer coreinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c, _, _, err := NewServiceController(config, serviceClient, informer, nil)
	if err != nil {
		klog.Errorf("failed to create multi cluster service controller %v", err)
		return
	}
	controllerManager.AddResourceSyncer(c)
}

func NewServiceController(config *config.SyncerConfiguration,
	serviceClient v1core.CoreV1Interface,
	informer coreinformers.Interface,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		serviceClient: serviceClient,
	}
	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	mcOptions.MaxConcurrentReconciles = constants.DwsControllerWorkerLow
	multiClusterServiceController, err := mc.NewMCController("tenant-masters-service-controller", &v1.Service{}, *mcOptions)
	if err != nil {
		return nil, nil, nil, err
	}
	c.multiClusterServiceController = multiClusterServiceController

	c.serviceLister = informer.Services().Lister()
	if options != nil && options.IsFake {
		c.serviceSynced = func() bool { return true }
	} else {
		c.serviceSynced = informer.Services().Informer().HasSynced
	}

	var uwOptions *uw.Options
	if options == nil || options.UWOptions == nil {
		uwOptions = &uw.Options{Reconciler: c}
	} else {
		uwOptions = options.UWOptions
	}
	uwOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerLow
	upwardServiceController, err := uw.NewUWController("service-upward-controller", &v1.Service{}, *uwOptions)
	if err != nil {
		klog.Errorf("failed to create service upward controller %v", err)
		return nil, nil, nil, err
	}
	c.upwardServiceController = upwardServiceController

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	servicePatroller, err := pa.NewPatroller("service-patroller", *patrolOptions)
	if err != nil {
		klog.Errorf("failed to create service patroller %v", err)
		return nil, nil, nil, err
	}
	c.servicePatroller = servicePatroller

	informer.Services().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Service:
					return isLoadBalancerService(t)
				case cache.DeletedFinalStateUnknown:
					if e, ok := t.Obj.(*v1.Service); ok {
						return isLoadBalancerService(e)
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %v to *v1.Service", obj))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in super master service controller: %v", obj))
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: c.enqueueService,
				UpdateFunc: func(oldObj, newObj interface{}) {
					newService := newObj.(*v1.Service)
					oldService := oldObj.(*v1.Service)
					if newService.ResourceVersion != oldService.ResourceVersion {
						c.enqueueService(newObj)
					}
				},
				DeleteFunc: c.enqueueService,
			},
		})
	return c, multiClusterServiceController, upwardServiceController, nil
}

func isLoadBalancerService(svc *v1.Service) bool {
	return svc.Spec.Type == v1.ServiceTypeLoadBalancer
}

func (c *controller) enqueueService(obj interface{}) {
	svc, ok := obj.(*v1.Service)
	if !ok {
		return
	}

	clusterName, _ := conversion.GetVirtualOwner(svc)
	if clusterName == "" {
		return
	}

	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %v: %v", obj, err))
		return
	}
	c.upwardServiceController.AddToQueue(key)
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-service-controller watch cluster %s for service resource", cluster.GetClusterName())
	err := c.multiClusterServiceController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s service event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-service-controller stop watching cluster %s for service resource", cluster.GetClusterName())
	c.multiClusterServiceController.TeardownClusterResource(cluster)
}
