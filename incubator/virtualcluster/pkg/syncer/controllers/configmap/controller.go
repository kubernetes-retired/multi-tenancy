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

package configmap

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"

	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type controller struct {
	configMapClient                 v1core.ConfigMapsGetter
	multiClusterConfigMapController *sc.MultiClusterController
}

func Register(
	configMapClient v1core.ConfigMapsGetter,
	configMapInformer coreinformers.ConfigMapInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		configMapClient: configMapClient,
	}

	// Create the multi cluster configmap controller
	options := sc.Options{Reconciler: c}
	multiClusterConfigMapController, err := sc.NewController("tenant-masters-configmap-controller", &v1.ConfigMap{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster configmap controller %v", err)
		return
	}
	c.multiClusterConfigMapController = multiClusterConfigMapController
	controllerManager.AddController(multiClusterConfigMapController)

	configMapInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.backPopulate,
		},
	)

	// Register the controller as cluster change listener
	listener.AddListener(c)
}

func (c *controller) backPopulate(obj interface{}) {
	configMap := obj.(*v1.ConfigMap)
	clusterName, namespace := conversion.GetOwner(configMap)
	if len(clusterName) == 0 {
		return
	}
	_, err := c.multiClusterConfigMapController.Get(clusterName, namespace, configMap.Name)
	if errors.IsNotFound(err) {
		return
	}
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile configmap %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileConfigMapCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.ConfigMap))
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, nil
		}
	case reconciler.UpdateEvent:
		err := c.reconcileConfigMapUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.ConfigMap))
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileConfigMapRemove(request.Cluster.Name, request.Namespace, request.Name)
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileConfigMapCreate(cluster, namespace, name string, configMap *v1.ConfigMap) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, configMap)
	if err != nil {
		return err
	}

	_, err = c.configMapClient.ConfigMaps(targetNamespace).Create(newObj.(*v1.ConfigMap))
	if errors.IsAlreadyExists(err) {
		klog.Infof("configmap %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcileConfigMapUpdate(cluster, namespace, name string, configMap *v1.ConfigMap) error {
	return nil
}

func (c *controller) reconcileConfigMapRemove(cluster, namespace, name string) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &conversion.DefaultDeletionPolicy,
	}
	err := c.configMapClient.ConfigMaps(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("configmap %s/%s of cluster %s not found in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster *cluster.Cluster) error {
	klog.Infof("tenant-masters-configmap-controller watch cluster %s for configmap resource", cluster.Name)
	err := c.multiClusterConfigMapController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s configmap event: %v", cluster.Name, err)
		return err
	}
	return nil
}

func (c *controller) RemoveCluster(cluster *cluster.Cluster) {
	klog.Warningf("not implemented yet")
}
