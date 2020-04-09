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
	"time"

	v1 "k8s.io/api/core/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type controller struct {
	// super master service client
	serviceClient v1core.ServicesGetter
	// super master informer/listers/synced functions
	serviceLister listersv1.ServiceLister
	serviceSynced cache.InformerSynced
	// Connect to all tenant master service informers
	multiClusterServiceController *mc.MultiClusterController
	// UWS queue
	workers int
	queue   workqueue.RateLimitingInterface
	// Checker timer
	periodCheckerPeriod time.Duration
}

func Register(
	config *config.SyncerConfiguration,
	serviceClient v1core.CoreV1Interface,
	informer coreinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c, _, err := NewServiceController(config, serviceClient, informer, nil)
	if err != nil {
		klog.Errorf("failed to create multi cluster service controller %v", err)
		return
	}
	controllerManager.AddController(c)
}

func NewServiceController(config *config.SyncerConfiguration, serviceClient v1core.CoreV1Interface, informer coreinformers.Interface, options *mc.Options) (manager.Controller, *mc.MultiClusterController, error) {
	c := &controller{
		serviceClient:       serviceClient,
		periodCheckerPeriod: 60 * time.Second,
		queue:               workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "super_master_service"),
		workers:             constants.UwsControllerWorkerLow,
	}

	if options == nil {
		options = &mc.Options{Reconciler: c}
	}
	options.MaxConcurrentReconciles = constants.DwsControllerWorkerLow
	multiClusterServiceController, err := mc.NewMCController("tenant-masters-service-controller", &v1.Service{}, *options)
	if err != nil {
		return nil, nil, err
	}
	c.multiClusterServiceController = multiClusterServiceController

	c.serviceLister = informer.Services().Lister()
	if options.IsFake {
		c.serviceSynced = func() bool { return true }
	} else {
		c.serviceSynced = informer.Services().Informer().HasSynced
	}

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
	return c, multiClusterServiceController, nil
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

	c.queue.Add(reconciler.UwsRequest{Key: key})
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
