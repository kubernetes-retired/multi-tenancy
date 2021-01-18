/*
Copyright 2021 The Kubernetes Authors.

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
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.endpointsSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.multiClusterEndpointsController.Start(stopCh)
}

// The reconcile logic for tenant master endpoints informer
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	vServiceObj, err := c.multiClusterEndpointsController.GetByObjectType(request.ClusterName, request.Namespace, request.Name, &v1.Service{})
	if err != nil && !errors.IsNotFound(err) {
		return reconciler.Result{Requeue: true}, fmt.Errorf("fail to query service from tenant master %s", request.ClusterName)
	}
	if err == nil {
		vService := vServiceObj.(*v1.Service)
		if vService.Spec.Selector != nil {
			// Supermaster ep controller handles the service ep lifecycle, quit.
			return reconciler.Result{}, nil
		}
	}
	klog.V(4).Infof("reconcile endpoints %s/%s for cluster %s", request.Namespace, request.Name, request.ClusterName)
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
		err := c.reconcileEndpointsCreate(request.ClusterName, targetNamespace, request.UID, vEndpoints)
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcileEndpointsRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pEndpoints)
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vEndpoints := vEndpointsObj.(*v1.Endpoints)
		err := c.reconcileEndpointsUpdate(request.ClusterName, targetNamespace, request.UID, pEndpoints, vEndpoints)
		if err != nil {
			klog.Errorf("failed reconcile endpoints %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileEndpointsCreate(clusterName, targetNamespace, requestUID string, ep *v1.Endpoints) error {
	vcName, vcNS, _, err := c.multiClusterEndpointsController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcNS, vcName, targetNamespace, ep)
	if err != nil {
		return err
	}

	pEndpoints := newObj.(*v1.Endpoints)

	pEndpoints, err = c.endpointClient.Endpoints(targetNamespace).Create(context.TODO(), pEndpoints, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		if pEndpoints.Annotations[constants.LabelUID] == requestUID {
			klog.Infof("endpoints %s/%s of cluster %s already exist in super master", targetNamespace, pEndpoints.Name, clusterName)
			return nil
		} else {
			return fmt.Errorf("pEndpoints %s/%s exists but its delegated object UID is different.", targetNamespace, pEndpoints.Name)
		}
	}
	return err
}

func (c *controller) reconcileEndpointsUpdate(clusterName, targetNamespace, requestUID string, pEP, vEP *v1.Endpoints) error {
	if pEP.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("pEndpoints %s/%s delegated UID is different from updated object.", targetNamespace, pEP.Name)
	}
	spec, err := util.GetVirtualClusterSpec(c.multiClusterEndpointsController, clusterName)
	if err != nil {
		return err
	}
	updatedEndpoints := conversion.Equality(c.config, spec).CheckEndpointsEquality(pEP, vEP)
	if updatedEndpoints != nil {
		pEP, err = c.endpointClient.Endpoints(targetNamespace).Update(context.TODO(), updatedEndpoints, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) reconcileEndpointsRemove(clusterName, targetNamespace, requestUID, name string, pEP *v1.Endpoints) error {
	if pEP.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pEndpoints %s/%s delegated UID is different from deleted object.", targetNamespace, pEP.Name)
	}
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.endpointClient.Endpoints(targetNamespace).Delete(context.TODO(), name, *opts)
	if errors.IsNotFound(err) {
		klog.Warningf("endpoints %s/%s of %s cluster not found in super master", targetNamespace, name, clusterName)
		return nil
	}
	return err
}
