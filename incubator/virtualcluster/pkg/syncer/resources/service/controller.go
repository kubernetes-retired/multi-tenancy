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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

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
	// Checker timer
	periodCheckerPeriod time.Duration
}

func Register(
	serviceClient v1core.ServicesGetter,
	serviceInformer coreinformers.ServiceInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		serviceClient:       serviceClient,
		periodCheckerPeriod: 60 * time.Second,
	}

	// Create the multi cluster service controller
	options := mc.Options{Reconciler: c}
	multiClusterServiceController, err := mc.NewMCController("tenant-masters-service-controller", &v1.Service{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster service controller %v", err)
		return
	}
	c.multiClusterServiceController = multiClusterServiceController

	c.serviceLister = serviceInformer.Lister()
	c.serviceSynced = serviceInformer.Informer().HasSynced

	controllerManager.AddController(c)
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return c.multiClusterServiceController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile service %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileServiceCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Service))
		if err != nil {
			klog.Errorf("failed reconcile service %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcileServiceUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Service))
		if err != nil {
			klog.Errorf("failed reconcile service %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileServiceRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Service))
		if err != nil {
			klog.Errorf("failed reconcile service %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileServiceCreate(cluster, namespace, name string, service *v1.Service) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, service)
	if err != nil {
		return err
	}

	pService := newObj.(*v1.Service)
	conversion.VC(nil).Service(pService).Mutate(service)

	_, err = c.serviceClient.Services(targetNamespace).Create(pService)
	if errors.IsAlreadyExists(err) {
		klog.Infof("service %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcileServiceUpdate(cluster, namespace, name string, service *v1.Service) error {
	return nil
}

func (c *controller) reconcileServiceRemove(cluster, namespace, name string, service *v1.Service) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.serviceClient.Services(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("service %s/%s of cluster not found in super master", namespace, name)
		return nil
	}
	return err
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
