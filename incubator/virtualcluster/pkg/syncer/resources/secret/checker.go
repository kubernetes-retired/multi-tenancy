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
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

// StartPeriodChecker starts the period checker for data consistency check. Checker is
// blocking so should be called via a goroutine.
func (c *controller) StartPeriodChecker(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stopCh, c.secretSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Secret checker")
	}

	// Start a loop to periodically check if secrets keep consistency between super
	// master and tenant masters.
	wait.Until(c.checkSecrets, c.periodCheckerPeriod, stopCh)

	return nil
}

// checkSecrets checks to see if pods in super master informer cache and tenant master
// keep consistency.
func (c *controller) checkSecrets() {
	clusterNames := c.multiClusterSecretController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	wg := sync.WaitGroup{}

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkSecretsOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pSecrets, err := c.secretLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing secrets from super master informer cache: %v", err)
		return
	}

	for _, pSecret := range pSecrets {
		clusterName, vNamespace := conversion.GetVirtualOwner(pSecret)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}

		_, err := c.multiClusterSecretController.Get(clusterName, vNamespace, pSecret.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				// vSecret not found and pSecret still exist, we need to delete pConfigMap manually
				klog.Info("vSecret %s/%s not found, delete corresponding pSecret manually", vNamespace, pSecret.Name)
				deleteOptions := &metav1.DeleteOptions{}
				deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pSecret.UID))
				if err = c.secretClient.Secrets(pSecret.Namespace).Delete(pSecret.Name, deleteOptions); err != nil {
					klog.Errorf("error deleting pConfigMap %v/%v in super master: %v", pSecret.Namespace, pSecret.Name, err)
				}
				continue
			}
			klog.Errorf("error getting vSecret %s/%s from cluster %s cache: %v", vNamespace, pSecret.Name, clusterName, err)
		}
	}
}

// checkSecretsOfTenantCluster checks to see if secrets in specific cluster keeps consistency.
func (c *controller) checkSecretsOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterSecretController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing secrets from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.Infof("check secrets consistency in cluster %s", clusterName)
	secretList := listObj.(*v1.SecretList)
	for i, vSecret := range secretList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vSecret.Namespace)
		_, err := c.secretLister.Secrets(targetNamespace).Get(vSecret.Name)
		if errors.IsNotFound(err) {
			// pSecret not found and vSecret still exists, we need to create pSecret again
			if err := c.multiClusterSecretController.RequeueObject(clusterName, &secretList.Items[i], reconciler.AddEvent); err != nil {
				klog.Errorf("error requeue vSecret %v/%v in cluster %s: %v", vSecret.Namespace, vSecret.Name, clusterName, err)
			}
			continue
		}

		if err != nil {
			klog.Errorf("error getting pSecret %s/%s from super master cache: %v", targetNamespace, vSecret.Name, err)
		}
	}
}
