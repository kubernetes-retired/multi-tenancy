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
	if !cache.WaitForCacheSync(stopCh, c.serviceSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service dws")
	}
	return c.multiClusterServiceController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile service %s/%s for cluster %s", request.Namespace, request.Name, request.ClusterName)
	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)
	pService, err := c.serviceLister.Services(targetNamespace).Get(request.Name)
	pExists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		pExists = false
	}
	vExists := true
	vServiceObj, err := c.multiClusterServiceController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}

	if vExists && !pExists {
		vService := vServiceObj.(*v1.Service)
		err := c.reconcileServiceCreate(request.ClusterName, targetNamespace, request.UID, vService)
		if err != nil {
			klog.Errorf("failed reconcile service %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcileServiceRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pService)
		if err != nil {
			klog.Errorf("failed reconcile service %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vService := vServiceObj.(*v1.Service)
		err := c.reconcileServiceUpdate(request.ClusterName, targetNamespace, request.UID, pService, vService)
		if err != nil {
			klog.Errorf("failed reconcile service %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileServiceCreate(clusterName, targetNamespace, requestUID string, service *v1.Service) error {
	newObj, err := conversion.BuildMetadata(clusterName, targetNamespace, service)
	if err != nil {
		return err
	}

	pService := newObj.(*v1.Service)
	conversion.VC(nil, "").Service(pService).Mutate(service)

	pService, err = c.serviceClient.Services(targetNamespace).Create(pService)
	if errors.IsAlreadyExists(err) {
		if pService.Annotations[constants.LabelUID] == requestUID {
			klog.Infof("service %s/%s of cluster %s already exist in super master", targetNamespace, pService.Name, clusterName)
			return nil
		} else {
			return fmt.Errorf("pService %s/%s exists but its delegated object UID is different.", targetNamespace, pService.Name)
		}
	}
	return err
}

func (c *controller) reconcileServiceUpdate(clusterName, targetNamespace, requestUID string, pService, vService *v1.Service) error {
	if pService.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("pService %s/%s delegated UID is different from updated object.", targetNamespace, pService.Name)
	}

	spec, err := c.multiClusterServiceController.GetSpec(clusterName)
	if err != nil {
		return err
	}
	updated := conversion.Equality(c.config, spec).CheckServiceEquality(pService, vService)
	if updated != nil {
		_, err = c.serviceClient.Services(targetNamespace).Update(updated)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) reconcileServiceRemove(clusterName, targetNamespace, requestUID, name string, pService *v1.Service) error {
	if pService.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pService %s/%s delegated UID is different from deleted object.", targetNamespace, name)
	}

	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
		Preconditions:     metav1.NewUIDPreconditions(string(pService.UID)),
	}
	err := c.serviceClient.Services(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("To be deleted service %s/%s not found in super master", targetNamespace, name)
		return nil
	}
	return err
}
