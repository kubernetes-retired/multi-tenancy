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
	"sync"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
)

var numMissMatchedOpaqueSecrets uint64
var numMissMatchedSASecrets uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.secretSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting secret checker")
	}
	c.secretPatroller.Start(stopCh)
	return nil
}

// PatrollerDo check if normal secrets and service account secrets keep consistency between super master and tenant masters.
func (c *controller) PatrollerDo() {
	clusterNames := c.multiClusterSecretController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up secret period checker")
		return
	}

	var wg sync.WaitGroup
	numMissMatchedOpaqueSecrets = 0
	numMissMatchedSASecrets = 0

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkSecretOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	secretList, err := c.secretLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing secret from super master informer cache: %v", err)
		return
	}

	klog.V(4).Infof("check secrets consistency in super")
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
		vSecretName := pSecret.Name
		if secretName := pSecret.GetAnnotations()[constants.LabelSecretName]; secretName != "" {
			vSecretName = secretName
		}
		// check whether secret is exists in tenant.
		vSecretObj, err := c.multiClusterSecretController.Get(clusterName, vNamespace, vSecretName)
		if errors.IsNotFound(err) {
			shouldDelete = true
		}
		if err == nil {
			vSecret := vSecretObj.(*v1.Secret)
			if pSecret.Annotations[constants.LabelUID] != string(vSecret.UID) {
				shouldDelete = true
				klog.Warningf("Found pSecret %s/%s delegated UID is different from tenant object.", pSecret.Namespace, pSecret.Name)
			}
		}

		if shouldDelete {
			deleteOptions := metav1.NewPreconditionDeleteOptions(string(pSecret.UID))
			if err := c.secretClient.Secrets(pSecret.Namespace).Delete(context.TODO(), pSecret.Name, *deleteOptions); err != nil {
				klog.Errorf("error deleting pSecret %s/%s in super master: %v", pSecret.Namespace, pSecret.Name, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterSecrets").Inc()
			}
		}
	}

	metrics.CheckerMissMatchStats.WithLabelValues("MissMatchedOpaqueSecrets").Set(float64(numMissMatchedOpaqueSecrets))
	metrics.CheckerMissMatchStats.WithLabelValues("MissMatchedSASecrets").Set(float64(numMissMatchedSASecrets))
}

func (c *controller) checkSecretOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterSecretController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing secrets from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.V(4).Infof("check secrets consistency in cluster %s", clusterName)
	secretList := listObj.(*v1.SecretList)
	for i, vSecret := range secretList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vSecret.Namespace)

		if vSecret.Type == v1.SecretTypeServiceAccountToken {
			c.checkServiceAccountTokenTypeSecretOfTenantCluster(clusterName, targetNamespace, &secretList.Items[i])
			continue
		}

		pSecret, err := c.secretLister.Secrets(targetNamespace).Get(vSecret.Name)
		if errors.IsNotFound(err) {
			if err := c.multiClusterSecretController.RequeueObject(clusterName, &secretList.Items[i]); err != nil {
				klog.Errorf("error requeue vSecret %v/%v in cluster %s: %v", vSecret.Namespace, vSecret.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantOpaqueSecrets").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("failed to get pSecret %s/%s from super master cache: %v", targetNamespace, vSecret.Name, err)
			continue
		}

		if pSecret.Annotations[constants.LabelUID] != string(vSecret.UID) {
			klog.Errorf("Found pSecret %s/%s delegated UID is different from tenant object.", targetNamespace, pSecret.Name)
			continue
		}
		spec, err := util.GetVirtualClusterSpec(c.multiClusterSecretController, clusterName)
		if err != nil {
			klog.Errorf("fail to get cluster spec : %s", clusterName)
			continue
		}

		updatedSecret := conversion.Equality(c.config, spec).CheckSecretEquality(pSecret, &secretList.Items[i])
		if updatedSecret != nil {
			atomic.AddUint64(&numMissMatchedOpaqueSecrets, 1)
			klog.Warningf("spec of secret %v/%v diff in super&tenant master", vSecret.Namespace, vSecret.Name)
		}
	}
}

func (c *controller) checkServiceAccountTokenTypeSecretOfTenantCluster(clusterName, targetNamespace string, vSecret *v1.Secret) {
	secretList, err := c.secretLister.Secrets(targetNamespace).List(labels.SelectorFromSet(map[string]string{
		constants.LabelSecretUID: string(vSecret.UID),
	}))
	if errors.IsNotFound(err) || len(secretList) == 0 {
		if err := c.multiClusterSecretController.RequeueObject(clusterName, vSecret); err != nil {
			klog.Errorf("error requeue service account type vSecret %v/%v in cluster %s: %v", vSecret.Namespace, vSecret.Name, clusterName, err)
		} else {
			metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantSASecrets").Inc()
		}
		return
	}

	if err != nil {
		klog.Errorf("failed to get service account token type pSecret %s/%s from super master cache: %v", targetNamespace, vSecret.Name, err)
		return
	}

	if len(secretList) > 1 {
		klog.Warningf("found service account token type pSecret %s/%s more than one", targetNamespace, vSecret.Name)
		return
	}
	if secretList[0].Annotations[constants.LabelUID] != string(vSecret.UID) {
		klog.Errorf("Found pSecret %s/%s delegated UID is different from tenant object.", targetNamespace, secretList[0].Name)
		return
	}
	spec, err := util.GetVirtualClusterSpec(c.multiClusterSecretController, clusterName)
	if err != nil {
		klog.Errorf("fail to get cluster spec : %s", clusterName)
		return
	}

	updatedSecret := conversion.Equality(c.config, spec).CheckSecretEquality(secretList[0], vSecret)
	if updatedSecret != nil {
		atomic.AddUint64(&numMissMatchedSASecrets, 1)
		klog.Warningf("spec of service account token type secret %v/%v diff in super&tenant master", vSecret.Namespace, vSecret.Name)
	}
}
