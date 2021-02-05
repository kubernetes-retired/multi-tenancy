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
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
)

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stopCh, c.saSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting SA checker")
	}
	c.Patroller.Start(stopCh)
	return nil
}

// PatrollerDo checks to see if serviceaccounts in super master informer cache and tenant master
// keep consistency.
func (c *controller) PatrollerDo() {
	clusterNames := c.MultiClusterController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	wg := sync.WaitGroup{}

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkServiceAccountsOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pServiceAccounts, err := c.saLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing serviceaccounts from super master informer cache: %v", err)
		return
	}

	for _, pSa := range pServiceAccounts {
		clusterName, vNamespace := conversion.GetVirtualOwner(pSa)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}
		shouldDelete := false
		vSaObj, err := c.MultiClusterController.Get(clusterName, vNamespace, pSa.Name)
		if errors.IsNotFound(err) {
			shouldDelete = true
		}
		if err == nil {
			vSa := vSaObj.(*v1.ServiceAccount)
			if pSa.Annotations[constants.LabelUID] != string(vSa.UID) {
				shouldDelete = true
				klog.Warningf("Found pServiceAccount %s/%s delegated UID is different from tenant object.", pSa.Namespace, pSa.Name)
			}
		}
		if shouldDelete {
			// vSa not found and pSa still exist, we need to delete pSa manually
			deleteOptions := &metav1.DeleteOptions{}
			deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pSa.UID))
			if err = c.saClient.ServiceAccounts(pSa.Namespace).Delete(context.TODO(), pSa.Name, *deleteOptions); err != nil {
				klog.Errorf("error deleting pServiceAccount %v/%v in super master: %v", pSa.Namespace, pSa.Name, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterServiceAccounts").Inc()
			}
		}
	}
}

// ccheckServiceAccountsOfTenantCluste checks to see if serviceaccounts in specific cluster keeps consistency.
func (c *controller) checkServiceAccountsOfTenantCluster(clusterName string) {
	listObj, err := c.MultiClusterController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing serviceaccounts from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.V(4).Infof("check serviceaccounts consistency in cluster %s", clusterName)
	saList := listObj.(*v1.ServiceAccountList)
	for i, vSa := range saList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vSa.Namespace)
		pSa, err := c.saLister.ServiceAccounts(targetNamespace).Get(vSa.Name)
		if errors.IsNotFound(err) {
			// pSa not found and vSa still exists, we need to create pSa again
			if err := c.MultiClusterController.RequeueObject(clusterName, &saList.Items[i]); err != nil {
				klog.Errorf("error requeue vServiceAccount %v/%v in cluster %s: %v", vSa.Namespace, vSa.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantServiceAccounts").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("error getting pServiceAccount %s/%s from super master cache: %v", targetNamespace, vSa.Name, err)
		}
		// Serviceaccounts are handled by sa controller in tenant/super master separately. The secrets of pSa and vSa are not expected to be equal
		if pSa.Annotations[constants.LabelUID] != string(vSa.UID) {
			klog.Warningf("Found pServiceAccount %s/%s delegated UID is different from tenant object.", pSa.Namespace, pSa.Name)
		}
	}
}
