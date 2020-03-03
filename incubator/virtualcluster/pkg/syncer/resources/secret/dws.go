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
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
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
	vSecretObj, err := c.multiClusterSecretController.Get(request.ClusterName, request.Namespace, request.Name)
	vExists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}
	// FIXME: Do we need to add sa name in the selector?
	pExists := true
	var pSecret *v1.Secret
	secretList, err := c.secretLister.Secrets(targetNamespace).List(labels.SelectorFromSet(map[string]string{
		constants.LabelSecretName: request.Name,
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
		// We need to use name to search again for normal vScrect
		pSecret, err = c.secretLister.Secrets(targetNamespace).Get(request.Name)
		if err != nil {
			if !errors.IsNotFound(err) {
				return reconciler.Result{Requeue: true}, err
			}
			pExists = false
		}
	}

	if vExists && !pExists {
		vSecret := vSecretObj.(*v1.Secret)
		err := c.reconcileSecretCreate(request.ClusterName, targetNamespace, request.UID, vSecret)
		if err != nil {
			klog.Errorf("failed reconcile secret %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcileSecretRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pSecret)
		if err != nil {
			klog.Errorf("failed reconcile secret %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vSecret := vSecretObj.(*v1.Secret)
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
	newObj, err := conversion.BuildMetadata(clusterName, targetNamespace, vSecret)
	if err != nil {
		return err
	}

	pSecret := newObj.(*v1.Secret)
	conversion.VC(c.multiClusterSecretController, "").ServiceAccountTokenSecret(pSecret).Mutate(vSecret, clusterName)

	_, err = c.secretClient.Secrets(targetNamespace).Create(pSecret)
	if errors.IsAlreadyExists(err) {
		klog.Infof("secret %s/%s of cluster %s already exist in super master", targetNamespace, pSecret.Name, clusterName)
		return nil
	}

	return err
}

func (c *controller) reconcileServiceAccountSecretUpdate(clusterName, targetNamespace string, pSecret, vSecret *v1.Secret) error {
	updatedBinaryData, equal := conversion.Equality(nil).CheckBinaryDataEquality(pSecret.Data, vSecret.Data)
	if equal {
		return nil
	}

	updatedSecret := pSecret.DeepCopy()
	updatedSecret.Data = updatedBinaryData
	_, err := c.secretClient.Secrets(targetNamespace).Update(updatedSecret)
	if err != nil {
		return err
	}

	return nil
}

func (c *controller) reconcileNormalSecretCreate(clusterName, targetNamespace, requestUID string, secret *v1.Secret) error {
	newObj, err := conversion.BuildMetadata(clusterName, targetNamespace, secret)
	if err != nil {
		return err
	}

	pSecret, err := c.secretClient.Secrets(targetNamespace).Create(newObj.(*v1.Secret))
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
	spec, err := c.multiClusterSecretController.GetSpec(clusterName)
	if err != nil {
		return err
	}
	updatedSecret := conversion.Equality(spec).CheckSecretEquality(pSecret, vSecret)
	if updatedSecret != nil {
		pSecret, err = c.secretClient.Secrets(targetNamespace).Update(updatedSecret)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *controller) reconcileSecretRemove(clusterName, targetNamespace, requestUID, name string, secret *v1.Secret) error {
	switch secret.Type {
	case v1.SecretTypeServiceAccountToken:
		return c.reconcileServiceAccountTokenSecretRemove(clusterName, targetNamespace, name)
	default:
		return c.reconcileNormalSecretRemove(clusterName, targetNamespace, requestUID, name, secret)
	}
}

func (c *controller) reconcileNormalSecretRemove(clusterName, targetNamespace, requestUID, name string, pSecret *v1.Secret) error {
	if pSecret.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pSecret %s/%s delegated UID is different from deleted object.", targetNamespace, pSecret.Name)
	}
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.secretClient.Secrets(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("secret %s/%s of cluster is not found in super master", targetNamespace, name)
		return nil
	}
	return err
}

func (c *controller) reconcileServiceAccountTokenSecretRemove(clusterName, targetNamespace, name string) error {
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.secretClient.Secrets(targetNamespace).DeleteCollection(opts, metav1.ListOptions{
		LabelSelector: labels.Set(map[string]string{
			constants.LabelSecretName: name,
		}).String(),
	})
	if errors.IsNotFound(err) {
		klog.Warningf("secret %s/%s of cluster is not found in super master", targetNamespace, name)
		return nil
	}
	return err
}
