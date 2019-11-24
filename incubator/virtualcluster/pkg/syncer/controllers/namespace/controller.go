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

package namespace

import (
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	ctrl "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type controller struct {
	namespaceClient                 v1core.NamespacesGetter
	multiClusterNamespaceController *sc.MultiClusterController
}

func Register(
	namespaceClient v1core.NamespacesGetter,
	namespaceInformer coreinformers.NamespaceInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		namespaceClient: namespaceClient,
	}

	// Create the multi cluster configmap controller
	options := sc.Options{Reconciler: c}
	multiClusterNamespaceController, err := sc.NewController("tenant-masters-namespace-controller", &v1.Namespace{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster namespace controller %v", err)
		return
	}
	c.multiClusterNamespaceController = multiClusterNamespaceController
	controllerManager.AddController(c)
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return c.multiClusterNamespaceController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile namespace %s %s event for cluster %s", request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileNamespaceCreate(request.Cluster.Name, request.Name, request.Obj.(*v1.Namespace))
		if err != nil {
			klog.Errorf("failed reconcile namespace %s CREATE of cluster %s %v", request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, nil
		}
	case reconciler.UpdateEvent:
		err := c.reconcileNamespaceUpdate(request.Cluster.Name, request.Name, request.Obj.(*v1.Namespace))
		if err != nil {
			klog.Errorf("failed reconcile namespace %s UPDATE of cluster %s %v", request.Name, request.Cluster.Name, err)
			return reconciler.Result{}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileNamespaceRemove(request.Cluster.Name, request.Name)
		if err != nil {
			klog.Errorf("failed reconcile namespace %s DELETE of cluster %s %v", request.Name, request.Cluster.Name, err)
			return reconciler.Result{}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileNamespaceCreate(cluster, name string, namespace *v1.Namespace) error {
	newObj, err := conversion.BuildSuperMasterNamespace(cluster, namespace)
	if err != nil {
		return err
	}

	_, err = c.namespaceClient.Namespaces().Create(newObj.(*v1.Namespace))
	if errors.IsAlreadyExists(err) {
		klog.Infof("namespace %s of cluster %s already exist in super master", name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcileNamespaceUpdate(cluster, name string, namespace *v1.Namespace) error {
	return nil
}

func (c *controller) reconcileNamespaceRemove(cluster, name string) error {
	targetName := strings.Join([]string{cluster, name}, "-")
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.namespaceClient.Namespaces().Delete(targetName, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("namespace %s of cluster %s not found in super master", name, cluster)
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster ctrl.ClusterInterface) {
	klog.Infof("tenant-masters-namespace-controller watch cluster %s for namespace resource", cluster.GetClusterName())
	err := c.multiClusterNamespaceController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s namespace event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster ctrl.ClusterInterface) {
	klog.Infof("tenant-masters-namespace-controller stop watching cluster %s for namespace resource", cluster.GetClusterName())
	c.multiClusterNamespaceController.TeardownClusterResource(cluster)
}
