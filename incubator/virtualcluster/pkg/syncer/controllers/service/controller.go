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
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	"k8s.io/apimachinery/pkg/api/equality"
)

type controller struct {
	serviceClient                 v1core.ServicesGetter
	multiClusterServiceController *sc.MultiClusterController
}

func Register(
	serviceClient v1core.ServicesGetter,
	serviceInformer coreinformers.ServiceInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		serviceClient: serviceClient,
	}

	// Create the multi cluster service controller
	options := sc.Options{Reconciler: c}
	multiClusterServiceController, err := sc.NewController("tenant-masters-service-controller", &v1.Service{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster service controller %v", err)
		return
	}
	c.multiClusterServiceController = multiClusterServiceController
	controllerManager.AddController(multiClusterServiceController)

	serviceInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.backPopulate,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newService := newObj.(*v1.Service)
				oldService := oldObj.(*v1.Service)
				if newService.ResourceVersion == oldService.ResourceVersion {
					// Periodic resync will send update events for all known Deployments.
					// Two different versions of the same Deployment will always have different RVs.
					return
				}

				c.backPopulate(newObj)
			},
		},
	)

	// Register the controller as cluster change listener
	listener.AddListener(c)
}

func (c *controller) backPopulate(obj interface{}) {
	service := obj.(*v1.Service)
	clusterName, namespace := conversion.GetOwner(service)
	if len(clusterName) == 0 {
		return
	}
	klog.Infof("back populate service %s/%s in cluster %s", service.Name, namespace, clusterName)
	vServiceObj, err := c.multiClusterServiceController.Get(clusterName, namespace, service.Name)
	if errors.IsNotFound(err) {
		klog.Errorf("could not find service %s/%s pod in controller cache %v", service.Name, namespace, err)
		return
	}
	var client *clientset.Clientset
	innerCluster := c.multiClusterServiceController.GetCluster(clusterName)
	client, err = clientset.NewForConfig(restclient.AddUserAgent(innerCluster.GetClientInfo().Config, "syncer"))
	if err != nil {
		return
	}

	vService := vServiceObj.(*v1.Service)
	if vService.Spec.ClusterIP != service.Spec.ClusterIP || !equality.Semantic.DeepEqual(vService.Spec.Ports, service.Spec.Ports) {
		newService := vService.DeepCopy()
		newService.Spec.ClusterIP = service.Spec.ClusterIP
		newService.Spec.Ports = service.Spec.Ports
		_, err = client.CoreV1().Services(vService.Namespace).Update(newService)
		if err != nil {
			klog.Errorf("failed to update service %s/%s of cluster %s %v", vService.Namespace, vService.Name, clusterName, err)
			return
		}
	}

	if !equality.Semantic.DeepEqual(vService.Status, service.Status) {
		newService := vService.DeepCopy()
		newService.Status = service.Status
		_, err = client.CoreV1().Services(vService.Namespace).UpdateStatus(newService)
		if err != nil {
			klog.Errorf("failed to update service %s/%s of cluster %s %v", vService.Namespace, vService.Name, clusterName, err)
			return
		}
	}
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
	conversion.MutateService(pService)

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
		PropagationPolicy: &conversion.DefaultDeletionPolicy,
	}
	err := c.serviceClient.Services(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("service %s/%s of cluster not found in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster *cluster.Cluster) {
	klog.Infof("tenant-masters-service-controller watch cluster %s for service resource", cluster.Name)
	err := c.multiClusterServiceController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s service event: %v", cluster.Name, err)
	}
}

func (c *controller) RemoveCluster(cluster *cluster.Cluster) {
	klog.Warningf("not implemented yet")
}
