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

package endpoints

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.endpointsSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.multiClusterEndpointsController.Start(stopCh)
}

// The reconcile logic for tenant master endpoints informer
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile endpoints %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileEndpointsCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Endpoints))
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcileEndpointsUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Endpoints))
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileEndpointsRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Endpoints))
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileEndpointsCreate(cluster, namespace, name string, ep *v1.Endpoints) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	_, err := c.endpointsLister.Endpoints(targetNamespace).Get(name)
	if err == nil {
		return c.reconcileEndpointsUpdate(cluster, namespace, name, ep)
	}

	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, ep)
	if err != nil {
		return err
	}

	pEndpoints := newObj.(*v1.Endpoints)

	_, err = c.endpointClient.Endpoints(targetNamespace).Create(pEndpoints)
	if errors.IsAlreadyExists(err) {
		klog.Infof("endpoints %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcileEndpointsUpdate(cluster, namespace, name string, vEP *v1.Endpoints) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	pEP, err := c.endpointsLister.Endpoints(targetNamespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	spec, err := c.multiClusterEndpointsController.GetSpec(cluster)
	if err != nil {
		return err
	}
	updatedEndpoints := conversion.Equality(spec).CheckEndpointsEquality(pEP, vEP)
	if updatedEndpoints != nil {
		pEP, err = c.endpointClient.Endpoints(targetNamespace).Update(updatedEndpoints)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *controller) reconcileEndpointsRemove(cluster, namespace, name string, ep *v1.Endpoints) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.endpointClient.Endpoints(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("endpoints %s/%s of cluster not found in super master", namespace, name)
		return nil
	}
	return err
}
