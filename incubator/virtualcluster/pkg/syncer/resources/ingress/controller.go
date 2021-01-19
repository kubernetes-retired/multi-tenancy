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

package ingress

import (
	"fmt"

	v1beta1 "k8s.io/api/extensions/v1beta1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1beta1extensions "k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	listersv1beta1 "k8s.io/client-go/listers/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

type controller struct {
	config *config.SyncerConfiguration
	// super master ingress client
	ingressClient v1beta1extensions.IngressesGetter
	// super master informer/listers/synced functions
	ingressLister listersv1beta1.IngressLister
	ingressSynced cache.InformerSynced
	// Connect to all tenant master ingress informers
	multiClusterIngressController *mc.MultiClusterController
	// UWcontroller
	upwardIngressController *uw.UpwardController
	// Periodic checker
	ingressPatroller *pa.Patroller
}

func NewIngressController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		config:        config,
		ingressClient: client.ExtensionsV1beta1(),
	}
	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	mcOptions.MaxConcurrentReconciles = constants.DwsControllerWorkerLow
	multiClusterIngressController, err := mc.NewMCController("tenant-masters-ingress-controller", &v1beta1.Ingress{}, *mcOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create ingress mc controller: %v", err)
	}
	c.multiClusterIngressController = multiClusterIngressController

	c.ingressLister = informer.Extensions().V1beta1().Ingresses().Lister()
	if options != nil && options.IsFake {
		c.ingressSynced = func() bool { return true }
	} else {
		c.ingressSynced = informer.Extensions().V1beta1().Ingresses().Informer().HasSynced
	}

	var uwOptions *uw.Options
	if options == nil || options.UWOptions == nil {
		uwOptions = &uw.Options{Reconciler: c}
	} else {
		uwOptions = options.UWOptions
	}
	uwOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerLow
	upwardIngressController, err := uw.NewUWController("ingress-upward-controller", &v1beta1.Ingress{}, *uwOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create ingress upward controller: %v", err)
	}
	c.upwardIngressController = upwardIngressController

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	ingressPatroller, err := pa.NewPatroller("ingress-patroller", &v1beta1.Ingress{}, *patrolOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create ingress patroller: %v", err)
	}
	c.ingressPatroller = ingressPatroller

	informer.Extensions().V1beta1().Ingresses().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1beta1.Ingress:
					return true
				case cache.DeletedFinalStateUnknown:
					if _, ok := t.Obj.(*v1beta1.Ingress); ok {
						return true
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %v to *v1beta1.Ingress", obj))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in super master ingress controller: %v", obj))
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: c.enqueueIngress,
				UpdateFunc: func(oldObj, newObj interface{}) {
					newIngress := newObj.(*v1beta1.Ingress)
					oldIngress := oldObj.(*v1beta1.Ingress)
					if newIngress.ResourceVersion != oldIngress.ResourceVersion {
						c.enqueueIngress(newObj)
					}
				},
				DeleteFunc: c.enqueueIngress,
			},
		})
	return c, multiClusterIngressController, upwardIngressController, nil
}

func (c *controller) enqueueIngress(obj interface{}) {
	svc, ok := obj.(*v1beta1.Ingress)
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
	c.upwardIngressController.AddToQueue(key)
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-ingress-controller watch cluster %s for ingress resource", cluster.GetClusterName())
	err := c.multiClusterIngressController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s ingress event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-ingress-controller stop watching cluster %s for ingress resource", cluster.GetClusterName())
	c.multiClusterIngressController.TeardownClusterResource(cluster)
}
