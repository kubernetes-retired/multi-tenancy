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
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

type controller struct {
	manager.BaseResourceSyncer
	// super master service client
	serviceClient v1core.ServicesGetter
	// super master informer/listers/synced functions
	serviceLister listersv1.ServiceLister
	serviceSynced cache.InformerSynced
}

func NewServiceController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options manager.ResourceSyncerOptions) (manager.ResourceSyncer, error) {
	c := &controller{
		BaseResourceSyncer: manager.BaseResourceSyncer{
			Config: config,
		},
		serviceClient: client.CoreV1(),
	}

	var err error
	c.MultiClusterController, err = mc.NewMCController(&v1.Service{}, &v1.ServiceList{}, c, mc.WithOptions(options.MCOptions))
	if err != nil {
		return nil, err
	}

	c.serviceLister = informer.Core().V1().Services().Lister()
	if options.IsFake {
		c.serviceSynced = func() bool { return true }
	} else {
		c.serviceSynced = informer.Core().V1().Services().Informer().HasSynced
	}

	c.UpwardController, err = uw.NewUWController(&v1.Service{}, c, uw.WithOptions(options.UWOptions))
	if err != nil {
		return nil, err
	}

	c.Patroller, err = pa.NewPatroller(&v1.Service{}, c, pa.WithOptions(options.PatrolOptions))
	if err != nil {
		return nil, err
	}

	informer.Core().V1().Services().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Service:
					return isBackPopulateService(t)
				case cache.DeletedFinalStateUnknown:
					if e, ok := t.Obj.(*v1.Service); ok {
						return isBackPopulateService(e)
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
	return c, nil
}

func isBackPopulateService(svc *v1.Service) bool {
	return svc.Spec.Type == v1.ServiceTypeLoadBalancer || svc.Spec.Type == v1.ServiceTypeClusterIP
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
	c.UpwardController.AddToQueue(key)
}
