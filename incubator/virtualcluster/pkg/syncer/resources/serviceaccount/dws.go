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
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.saSynced) {
		return fmt.Errorf("failed to wait for sa caches to sync")
	}
	return c.multiClusterServiceAccountController.Start(stopCh)
}

// The reconcile logic for tenant master service account informer
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile service account %s/%s for cluster %s", request.Namespace, request.Name, request.ClusterName)
	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)
	pSa, err := c.saLister.ServiceAccounts(targetNamespace).Get(request.Name)
	pExists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		pExists = false
	}
	vExists := true
	vSaObj, err := c.multiClusterServiceAccountController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}

	if vExists && !pExists {
		vSa := vSaObj.(*v1.ServiceAccount)
		err := c.reconcileServiceAccountCreate(request.ClusterName, targetNamespace, request.UID, vSa)
		if err != nil {
			klog.Errorf("failed reconcile serviceaccount %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcileServiceAccountRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pSa)
		if err != nil {
			klog.Errorf("failed reconcile serviceaccount %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vSa := vSaObj.(*v1.ServiceAccount)
		err := c.reconcileServiceAccountUpdate(request.ClusterName, targetNamespace, request.UID, pSa, vSa)
		if err != nil {
			klog.Errorf("failed reconcile serviceaccount %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileServiceAccountCreate(clusterName, targetNamespace, requestUID string, vSa *v1.ServiceAccount) error {
	vcName, vcNS, _, err := c.multiClusterServiceAccountController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcNS, vcName, targetNamespace, vSa)
	if err != nil {
		return err
	}
	pServiceAccount := newObj.(*v1.ServiceAccount)
	// set to empty and token controller will regenerate one.
	pServiceAccount.Secrets = nil

	pServiceAccount, err = c.saClient.ServiceAccounts(targetNamespace).Create(context.TODO(), pServiceAccount, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		if pServiceAccount.Annotations[constants.LabelUID] == requestUID {
			klog.Infof("service account %s/%s of cluster %s already exist in super master", targetNamespace, pServiceAccount.Name, clusterName)
			return nil
		} else {
			return fmt.Errorf("pServiceAccount %s/%s exists but its delegated UID is different", targetNamespace, pServiceAccount.Name)
		}
	}
	return err
}

func (c *controller) reconcileServiceAccountUpdate(clusterName, targetNamespace, requestUID string, pSa, vSa *v1.ServiceAccount) error {
	// Just mark the default service account of super master namespace, created by super master service account controller, as a tenant related resource.
	if vSa.Name == "default" {
		if len(pSa.Annotations) == 0 {
			pSa.Annotations = make(map[string]string)
		}
		var err error
		if pSa.Annotations[constants.LabelCluster] != clusterName || pSa.Annotations[constants.LabelUID] != string(vSa.UID) || pSa.Annotations[constants.LabelNamespace] != vSa.Namespace {
			pSa.Annotations[constants.LabelCluster] = clusterName
			pSa.Annotations[constants.LabelUID] = string(vSa.UID)
			pSa.Annotations[constants.LabelNamespace] = vSa.Namespace
			_, err = c.saClient.ServiceAccounts(targetNamespace).Update(context.TODO(), pSa, metav1.UpdateOptions{})
		}
		return err
	}

	if pSa.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("pServiceAccount %s/%s delegated UID is different from updated object", targetNamespace, pSa.Name)
	}

	// do nothing.
	return nil
}

func (c *controller) reconcileServiceAccountRemove(clusterName, targetNamespace, requestUID, name string, pSa *v1.ServiceAccount) error {
	if pSa.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pServiceAccount %s/%s delegated UID is different from deleted object.", targetNamespace, pSa.Name)
	}
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.saClient.ServiceAccounts(targetNamespace).Delete(context.TODO(), name, *opts)
	if errors.IsNotFound(err) {
		klog.Warningf("service account %s/%s of cluster %s not found in super master", targetNamespace, name, clusterName)
		return nil
	}
	return err
}
