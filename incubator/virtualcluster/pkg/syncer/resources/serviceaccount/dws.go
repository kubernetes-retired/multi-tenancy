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
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.saSynced) {
		return fmt.Errorf("failed to wait for sa caches to sync")
	}
	return c.multiClusterServiceAccountController.Start(stopCh)
}

// The reconcile logic for tenant master service account informer
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile service account %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

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
	// Just mark the default service account of super master namespace, created by super master service account controller, as a tenant related resource.
	if name == "default" {
		sa, err := c.saLister.ServiceAccounts(targetNamespace).Get("default")
		if err != nil {
			// maybe the sa is not created, retry
			return err
		}

		if len(sa.Annotations) == 0 {
			sa.Annotations = make(map[string]string)
		}
		if sa.Annotations[constants.LabelCluster] != cluster {
			sa.Annotations[constants.LabelCluster] = cluster
			_, err = c.saClient.ServiceAccounts(targetNamespace).Update(sa)
		}
		return err
	}

	_, err := c.saLister.ServiceAccounts(targetNamespace).Get(name)
	if err == nil {
		// sa already exists.
		return nil
	}

	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, secret)
	if err != nil {
		return err
	}
	pServiceAccount := newObj.(*v1.ServiceAccount)
	// set to empty and token controller will regenerate one.
	pServiceAccount.Secrets = nil

	_, err = c.saClient.ServiceAccounts(targetNamespace).Create(pServiceAccount)
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
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.saClient.ServiceAccounts(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("service account %s/%s of cluster %s not found in super master", namespace, name, cluster)
		return nil
	}
	return err
}
