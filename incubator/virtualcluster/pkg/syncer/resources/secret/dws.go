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

package secret

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.secretSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.multiClusterSecretController.Start(stopCh)
}

// The reconcile logic for tenant master secret informer
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile secret %s/%s for cluster %s", request.Namespace, request.Name, request.ClusterName)
	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)
	var vSecret *v1.Secret
	vSecretObj, err := c.multiClusterSecretController.Get(request.ClusterName, request.Namespace, request.Name)
	if err == nil {
		vSecret = vSecretObj.(*v1.Secret)
	} else if !errors.IsNotFound(err) {
		return reconciler.Result{Requeue: true}, err
	}

	var pSecret *v1.Secret
	secretList, err := c.secretLister.Secrets(targetNamespace).List(labels.SelectorFromSet(map[string]string{
		constants.LabelSecretUID: request.UID,
	}))
	if err != nil && !errors.IsNotFound(err) {
		return reconciler.Result{Requeue: true}, err
	}
	if len(secretList) != 0 {
		// This is service account vSecret, it is unlikely we have a dup name in super but
		for i, each := range secretList {
			if each.Annotations[constants.LabelUID] == request.UID {
				pSecret = secretList[i]
				break
			}
		}
		if pSecret == nil {
			return reconciler.Result{Requeue: true}, fmt.Errorf("There are pSecrets that represent vSerect %s/%s but the UID is unmatched.", request.Namespace, request.Name)
		}
	} else {
		// We need to use name to search again for normal vSecret
		pSecret, err = c.secretLister.Secrets(targetNamespace).Get(request.Name)
		if err == nil {
			if pSecret.Type == v1.SecretTypeServiceAccountToken {
				pSecret = nil
			}
		} else if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
	}

	if vSecret != nil && pSecret == nil {
		err := c.reconcileSecretCreate(request.ClusterName, targetNamespace, request.UID, vSecret)
		if err != nil {
			klog.Errorf("failed reconcile secret %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vSecret == nil && pSecret != nil {
		err := c.reconcileSecretRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pSecret)
		if err != nil {
			klog.Errorf("failed reconcile secret %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vSecret != nil && pSecret != nil {
		err := c.reconcileSecretUpdate(request.ClusterName, targetNamespace, request.UID, pSecret, vSecret)
		if err != nil {
			klog.Errorf("failed reconcile secret %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileSecretCreate(clusterName, targetNamespace, requestUID string, secret *v1.Secret) error {
	switch secret.Type {
	case v1.SecretTypeServiceAccountToken:
		return c.reconcileServiceAccountSecretCreate(clusterName, targetNamespace, secret)
	default:
		return c.reconcileNormalSecretCreate(clusterName, targetNamespace, requestUID, secret)
	}
}

func (c *controller) reconcileServiceAccountSecretCreate(clusterName, targetNamespace string, vSecret *v1.Secret) error {
	vcName, vcNS, _, err := c.multiClusterSecretController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcNS, vcName, targetNamespace, vSecret)
	if err != nil {
		return err
	}

	pSecret := newObj.(*v1.Secret)
	conversion.VC(c.multiClusterSecretController, "").ServiceAccountTokenSecret(pSecret).Mutate(vSecret, clusterName)

	_, err = c.secretClient.Secrets(targetNamespace).Create(context.TODO(), pSecret, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		klog.Infof("secret %s/%s of cluster %s already exist in super master", targetNamespace, pSecret.Name, clusterName)
		return nil
	}

	return err
}

func (c *controller) reconcileServiceAccountSecretUpdate(clusterName, targetNamespace string, pSecret, vSecret *v1.Secret) error {
	updatedBinaryData, equal := conversion.Equality(c.config, nil).CheckBinaryDataEquality(pSecret.Data, vSecret.Data)
	if equal {
		return nil
	}

	updatedSecret := pSecret.DeepCopy()
	updatedSecret.Data = updatedBinaryData
	_, err := c.secretClient.Secrets(targetNamespace).Update(context.TODO(), updatedSecret, metav1.UpdateOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (c *controller) reconcileNormalSecretCreate(clusterName, targetNamespace, requestUID string, secret *v1.Secret) error {
	vcName, vcNS, _, err := c.multiClusterSecretController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcNS, vcName, targetNamespace, secret)
	if err != nil {
		return err
	}

	pSecret, err := c.secretClient.Secrets(targetNamespace).Create(context.TODO(), newObj.(*v1.Secret), metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		if pSecret.Annotations[constants.LabelUID] == requestUID {
			klog.Infof("secret %s/%s of cluster %s already exist in super master", targetNamespace, secret.Name, clusterName)
			return nil
		} else {
			return fmt.Errorf("pSecret %s/%s exists but its delegated object UID is different.", targetNamespace, pSecret.Name)
		}
	}

	return err
}

func (c *controller) reconcileSecretUpdate(clusterName, targetNamespace, requestUID string, pSecret, vSecret *v1.Secret) error {
	switch vSecret.Type {
	case v1.SecretTypeServiceAccountToken:
		return c.reconcileServiceAccountSecretUpdate(clusterName, targetNamespace, pSecret, vSecret)
	default:
		return c.reconcileNormalSecretUpdate(clusterName, targetNamespace, requestUID, pSecret, vSecret)
	}
}

func (c *controller) reconcileNormalSecretUpdate(clusterName, targetNamespace, requestUID string, pSecret, vSecret *v1.Secret) error {
	if pSecret.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("pEndpoints %s/%s delegated UID is different from updated object.", targetNamespace, pSecret.Name)
	}
	spec, err := util.GetVirtualClusterSpec(c.multiClusterSecretController, clusterName)
	if err != nil {
		return err
	}
	updatedSecret := conversion.Equality(c.config, spec).CheckSecretEquality(pSecret, vSecret)
	if updatedSecret != nil {
		pSecret, err = c.secretClient.Secrets(targetNamespace).Update(context.TODO(), updatedSecret, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *controller) reconcileSecretRemove(clusterName, targetNamespace, requestUID, name string, secret *v1.Secret) error {
	if _, isSaSecret := secret.Labels[constants.LabelSecretUID]; isSaSecret {
		return c.reconcileServiceAccountTokenSecretRemove(clusterName, targetNamespace, requestUID, name)
	}
	return c.reconcileNormalSecretRemove(clusterName, targetNamespace, requestUID, name, secret)
}

func (c *controller) reconcileNormalSecretRemove(clusterName, targetNamespace, requestUID, name string, pSecret *v1.Secret) error {
	if pSecret.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pSecret %s/%s delegated UID is different from deleted object.", targetNamespace, pSecret.Name)
	}
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.secretClient.Secrets(targetNamespace).Delete(context.TODO(), name, *opts)
	if errors.IsNotFound(err) {
		klog.Warningf("secret %s/%s of cluster is not found in super master", targetNamespace, name)
		return nil
	}
	return err
}

func (c *controller) reconcileServiceAccountTokenSecretRemove(clusterName, targetNamespace, requestUID, name string) error {
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.secretClient.Secrets(targetNamespace).DeleteCollection(context.TODO(), *opts, metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			constants.LabelSecretUID: requestUID,
		}).String(),
	})
	if errors.IsNotFound(err) {
		klog.Warningf("secret %s/%s of cluster is not found in super master", targetNamespace, name)
		return nil
	}
	return err
}
