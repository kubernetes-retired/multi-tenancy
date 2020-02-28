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
	if request.Namespace != "default" || request.Name != "kubernetes" {
		// For now, we bypass all ep events beside the default kubernetes ep. The tenant/master ep controllers handle ep lifecycle independently.
		return reconciler.Result{}, nil
	}
	klog.Infof("reconcile endpoints %s/%s for cluster %s", request.Namespace, request.Name, request.ClusterName)
	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)
	pEndpoints, err := c.endpointsLister.Endpoints(targetNamespace).Get(request.Name)
	pExists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		pExists = false
	}
	vExists := true
	vEndpointsObj, err := c.multiClusterEndpointsController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}

	if vExists && !pExists {
		vEndpoints := vEndpointsObj.(*v1.Endpoints)
		err := c.reconcileEndpointsCreate(request.ClusterName, targetNamespace, vEndpoints)
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcileEndpointsRemove(request.ClusterName, targetNamespace, request.Name)
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vEndpoints := vEndpointsObj.(*v1.Endpoints)
		err := c.reconcileEndpointsUpdate(request.ClusterName, targetNamespace, pEndpoints, vEndpoints)
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileEndpointsCreate(clusterName, targetNamespace string, ep *v1.Endpoints) error {
	newObj, err := conversion.BuildMetadata(clusterName, targetNamespace, ep)
	if err != nil {
		return err
	}

	pEndpoints := newObj.(*v1.Endpoints)

	_, err = c.endpointClient.Endpoints(targetNamespace).Create(pEndpoints)
	if errors.IsAlreadyExists(err) {
		klog.Infof("endpoints %s/%s of cluster %s already exist in super master", targetNamespace, pEndpoints.Name, clusterName)
		return nil
	}
	return err
}

func (c *controller) reconcileEndpointsUpdate(clusterName, targetNamespace string, pEP, vEP *v1.Endpoints) error {
	spec, err := c.multiClusterEndpointsController.GetSpec(clusterName)
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

func (c *controller) reconcileEndpointsRemove(clusterName, targetNamespace, name string) error {
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.endpointClient.Endpoints(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("endpoints %s/%s of %s cluster not found in super master", targetNamespace, name, clusterName)
		return nil
	}
	return err
}
