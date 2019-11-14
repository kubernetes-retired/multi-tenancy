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

package serviceaccount

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type controller struct {
	client                               v1core.CoreV1Interface
	saInformer                           coreinformers.ServiceAccountInformer
	multiClusterServiceAccountController *sc.MultiClusterController
}

func Register(
	client v1core.CoreV1Interface,
	saInformer coreinformers.ServiceAccountInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		client: client,
	}

	// Create the multi cluster secret controller
	options := sc.Options{Reconciler: c}
	multiClusterSecretController, err := sc.NewController("tenant-masters-service-account-controller", &v1.ServiceAccount{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster secret controller %v", err)
		return
	}
	c.multiClusterServiceAccountController = multiClusterSecretController
	controllerManager.AddController(multiClusterSecretController)

	// Register the controller as cluster change listener
	listener.AddListener(c)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile service account %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileServiceAccountCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.ServiceAccount))
		if err != nil {
			klog.Errorf("failed reconcile service account %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcileServiceAccountUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.ServiceAccount))
		if err != nil {
			klog.Errorf("failed reconcile service account %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileServiceAccountRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.ServiceAccount))
		if err != nil {
			klog.Errorf("failed reconcile service account %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileServiceAccountCreate(cluster, namespace, name string, secret *v1.ServiceAccount) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	// just mark the default service account as a tenant related resource.
	// service account controller will create it for us.
	if name == "default" {
		sa, err := c.client.ServiceAccounts(targetNamespace).Get("default", metav1.GetOptions{})
		if err != nil {
			// maybe the sa is not created, retry
			return err
		}

		if len(sa.Annotations) == 0 {
			sa.Annotations = make(map[string]string)
		}
		sa.Annotations[conversion.LabelCluster] = cluster
		_, err = c.client.ServiceAccounts(targetNamespace).Update(sa)
		return err
	}
	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, secret)
	if err != nil {
		return err
	}

	pServiceAccount := newObj.(*v1.ServiceAccount)
	// set to empty and token controller will regenerate one.
	pServiceAccount.Secrets = nil

	_, err = c.client.ServiceAccounts(targetNamespace).Create(pServiceAccount)
	if errors.IsAlreadyExists(err) {
		klog.Infof("service account %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcileServiceAccountUpdate(cluster, namespace, name string, secret *v1.ServiceAccount) error {
	// do nothing.
	return nil
}

func (c *controller) reconcileServiceAccountRemove(cluster, namespace, name string, secret *v1.ServiceAccount) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &conversion.DefaultDeletionPolicy,
	}
	err := c.client.ServiceAccounts(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("service account %s/%s of cluster not found in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster *cluster.Cluster) {
	klog.Infof("tenant-masters-service-account-controller watch cluster %s for secret resource", cluster.Name)
	err := c.multiClusterServiceAccountController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s secret event: %v", cluster.Name, err)
	}
}

func (c *controller) RemoveCluster(cluster *cluster.Cluster) {
	klog.Warningf("not implemented yet")
}
