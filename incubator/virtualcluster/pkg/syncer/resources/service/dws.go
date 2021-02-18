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

package service

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.serviceSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service dws")
	}
	return c.MultiClusterController.Start(stopCh)
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
	vServiceObj, err := c.MultiClusterController.Get(request.ClusterName, request.Namespace, request.Name)
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
	vcName, vcNS, _, err := c.MultiClusterController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcNS, vcName, targetNamespace, service)
	if err != nil {
		return err
	}

	pService := newObj.(*v1.Service)
	conversion.VC(nil, "").Service(pService).Mutate(service)

	pService, err = c.serviceClient.Services(targetNamespace).Create(context.TODO(), pService, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		if pService.Annotations[constants.LabelUID] == requestUID {
			if adoptableService(pService) {
				return c.reconcileServiceUnlinked(clusterName, service)
			} else {
				klog.Infof("service %s/%s of cluster %s already exist in super master", targetNamespace, pService.Name, clusterName)
				return nil
			}
		} else {
			return fmt.Errorf("pService %s/%s exists but its delegated object UID is different.", targetNamespace, pService.Name)
		}
	}
	return err
}

func (c *controller) reconcileServiceUpdate(clusterName, targetNamespace, requestUID string, pService, vService *v1.Service) error {
	if pService.Annotations[constants.LabelUID] != requestUID {
		// When a supercluster service is adoptable and doesn't have a UID
		// we add fire off thr missing UID checker to adopt the service
		if adoptableService(pService) {
			if err := c.reconcileServiceMissingUID(pService, vService); err != nil {
				klog.Errorf("error deleting pService %s/%s in super master: %v", pService.Namespace, pService.Name, err)
			}
		} else {
			return fmt.Errorf("pService %s/%s delegated UID is different from updated object.", targetNamespace, pService.Name)
		}
	}

	vc, err := util.GetVirtualClusterObject(c.MultiClusterController, clusterName)
	if err != nil {
		return err
	}
	updated := conversion.Equality(c.Config, vc).CheckServiceEquality(pService, vService)
	if updated != nil {
		_, err = c.serviceClient.Services(targetNamespace).Update(context.TODO(), updated, metav1.UpdateOptions{})
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
	err := c.serviceClient.Services(targetNamespace).Delete(context.TODO(), name, *opts)
	if errors.IsNotFound(err) {
		klog.Warningf("To be deleted service %s/%s not found in super master", targetNamespace, name)
		return nil
	}
	return err
}

func (c *controller) reconcileServiceMissingUID(pService, vService *v1.Service) error {
	// We need to add the UID to this object
	klog.Infof("Found pre-synced service without UID updating pService %s/%s in super master related UID: %v", pService.Namespace, pService.Name, vService.UID)
	pServiceCopy := pService.DeepCopy()
	pServiceCopy.Annotations[constants.LabelUID] = string(vService.UID)
	if _, err := c.serviceClient.Services(pService.Namespace).Update(context.TODO(), pServiceCopy, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("error updating pService %s/%s in super master: %v", pService.Namespace, pServiceCopy.Name, err)
	}

	metrics.CheckerRemedyStats.WithLabelValues("AdoptedSuperMasterService").Inc()
	return nil
}

func (c *controller) reconcileServiceUnlinked(clusterName string, vService *v1.Service) error {
	vCopy := vService.DeepCopy()
	// reset uid and other metadata
	vCopy.UID = types.UID("")
	vCopy.CreationTimestamp = metav1.Now()
	vCopy.Generation = 0
	vCopy.ManagedFields = []metav1.ManagedFieldsEntry{}
	vCopy.SelfLink = ""

	// Delete service
	client, err := c.MultiClusterController.GetClusterClient(clusterName)
	if err != nil {
		return err
	}

	if err := client.CoreV1().Services(vCopy.GetNamespace()).Delete(context.TODO(), vCopy.GetName(), metav1.DeleteOptions{}); err != nil {
		return err
	}

	retrier := func(err error) bool {
		if err != nil {
			return true
		}
		return false
	}

	// Recreate service
	return retry.OnError(retry.DefaultRetry, retrier, func() error {
		svc, err := client.CoreV1().Services(vCopy.GetNamespace()).Create(context.TODO(), vCopy, metav1.CreateOptions{})
		if errors.IsAlreadyExists(err) && string(svc.UID) == string(vService.UID) {
			return err
		} else if err != nil {
			return err
		} else {
			return nil
		}
	})
}
