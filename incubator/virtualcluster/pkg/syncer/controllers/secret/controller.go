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

package secret

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
	secretClient                 v1core.SecretsGetter
	multiClusterSecretController *sc.MultiClusterController
}

func Register(
	secretClient v1core.SecretsGetter,
	secretInformer coreinformers.SecretInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		secretClient: secretClient,
	}

	// Create the multi cluster secret controller
	options := sc.Options{Reconciler: c}
	multiClusterSecretController, err := sc.NewController("tenant-masters-secret-controller", &v1.Secret{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster secret controller %v", err)
		return
	}
	c.multiClusterSecretController = multiClusterSecretController
	controllerManager.AddController(multiClusterSecretController)

	secretInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.backPopulate,
		},
	)

	// Register the controller as cluster change listener
	listener.AddListener(c)
}

func (c *controller) backPopulate(obj interface{}) {
	secret := obj.(*v1.Secret)
	clusterName, namespace := conversion.GetOwner(secret)
	if len(clusterName) == 0 {
		return
	}
	_, err := c.multiClusterSecretController.Get(clusterName, namespace, secret.Name)
	if errors.IsNotFound(err) {
		return
	}
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile secret %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileSecretCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Secret))
		if err != nil {
			klog.Errorf("failed reconcile secret %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcileSecretUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Secret))
		if err != nil {
			klog.Errorf("failed reconcile secret %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileSecretRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Secret))
		if err != nil {
			klog.Errorf("failed reconcile secret %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileSecretCreate(cluster, namespace, name string, secret *v1.Secret) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, secret)
	if err != nil {
		return err
	}

	_, err = c.secretClient.Secrets(targetNamespace).Create(newObj.(*v1.Secret))
	if errors.IsAlreadyExists(err) {
		klog.Infof("secret %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcileSecretUpdate(cluster, namespace, name string, secret *v1.Secret) error {
	return nil
}

func (c *controller) reconcileSecretRemove(cluster, namespace, name string, secret *v1.Secret) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &conversion.DefaultDeletionPolicy,
	}
	err := c.secretClient.Secrets(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("secret %s/%s of cluster is not found in super master", namespace, name)
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster *cluster.Cluster) error {
	klog.Infof("tenant-masters-secret-controller watch cluster %s for secret resource", cluster.Name)
	err := c.multiClusterSecretController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s secret event: %v", cluster.Name, err)
		return err
	}
	return nil
}

func (c *controller) RemoveCluster(cluster *cluster.Cluster) {
	klog.Warningf("not implemented yet")
}
