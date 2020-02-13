/*
Copyright 2020 The Kubernetes Authors.

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
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func (c *controller) StartPeriodChecker(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.secretSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting secret checker")
	}

	wait.Until(c.checkSecrets, c.periodCheckerPeriod, stopCh)
	return nil
}

func (c *controller) checkSecrets() {
	clusterNames := c.multiClusterSecretController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up secret period checker")
		return
	}

	var wg sync.WaitGroup
	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkNormalSecretOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	secretList, err := c.secretLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing secret from super master informer cache: %v", err)
		return
	}

	klog.Infof("check secrets consistency in super")
	for _, pSecret := range secretList {
		// service account token type secret are managed by super individually.
		if pSecret.Type == v1.SecretTypeServiceAccountToken {
			continue
		}

		clusterName, vNamespace := conversion.GetVirtualOwner(pSecret)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}

		shouldDelete := false

		// virtual service account token type secret
		if saName := pSecret.GetAnnotations()[v1.ServiceAccountNameKey]; saName != "" {
			vSecretName := pSecret.GetLabels()[constants.LabelSecretName]
			// check whether service account token type secret is exists in tenant.
			tenantClient, err := c.multiClusterSecretController.GetClusterClient(clusterName)
			if err != nil {
				klog.Errorf("failed to get tenant client %s", clusterName)
				continue
			}
			_, err = tenantClient.CoreV1().Secrets(vNamespace).Get(vSecretName, metav1.GetOptions{})
			if errors.IsNotFound(err) {
				shouldDelete = true
			}
		} else {
			// check whether secret is exists in tenant.
			_, err := c.multiClusterSecretController.Get(clusterName, vNamespace, pSecret.Name)
			if errors.IsNotFound(err) {
				shouldDelete = true
			}
		}

		if shouldDelete {
			deleteOptions := metav1.NewPreconditionDeleteOptions(string(pSecret.UID))
			if err := c.secretClient.Secrets(pSecret.Namespace).Delete(pSecret.Name, deleteOptions); err != nil {
				klog.Errorf("error deleting pSecret %s/%s in super master: %v", pSecret.Namespace, pSecret.Name, err)
			}
		}
	}
}

func (c *controller) checkNormalSecretOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterSecretController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing secrets from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.Infof("check secrets consistency in cluster %s", clusterName)
	secretList := listObj.(*v1.SecretList)
	for i, vSecret := range secretList.Items {
		if vSecret.Type == v1.SecretTypeServiceAccountToken {
			continue
		}

		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vSecret.Namespace)
		pSecret, err := c.secretLister.Secrets(targetNamespace).Get(vSecret.Name)
		if errors.IsNotFound(err) {
			if err := c.multiClusterSecretController.RequeueObject(clusterName, &secretList.Items[i], reconciler.AddEvent); err != nil {
				klog.Errorf("error requeue vSecret %v/%v in cluster %s: %v", vSecret.Namespace, vSecret.Name, clusterName, err)
			}
			continue
		}

		if err != nil {
			klog.Errorf("failed to get pSecret %s/%s from super master cache: %v", targetNamespace, vSecret.Name, err)
			continue
		}

		spec, err := c.multiClusterSecretController.GetSpec(clusterName)
		if err != nil {
			klog.Errorf("fail to get cluster spec : %s", clusterName)
			continue
		}

		updatedSecret := conversion.Equality(spec).CheckSecretEquality(pSecret, &secretList.Items[i])
		if updatedSecret != nil {
			klog.Warningf("spec of secret %v/%v diff in super&tenant master", vSecret.Namespace, vSecret.Name)
		}
	}
}
