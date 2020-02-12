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

package namespace

import (
	"fmt"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
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

	if !cache.WaitForCacheSync(stopCh, c.nsSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Namespace checker")
	}

	// Start a loop to periodically check if namespaces keep consistency between super
	// master and tenant masters.
	wait.Until(c.checkNamespaces, c.periodCheckerPeriod, stopCh)

	return nil
}

// checkNamespaces checks to see if namespaces in super master informer cache and tenant master
// keep consistency.
func (c *controller) checkNamespaces() {
	clusterNames := c.multiClusterNamespaceController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	wg := sync.WaitGroup{}

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkNamespacesOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pNamespaces, err := c.nsLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing namespaces from super master informer cache: %v", err)
		return
	}

	for _, pNamespace := range pNamespaces {
		clusterName, vNamespace, _ := conversion.GetVirtualNamespace(c.nsLister, pNamespace.Name)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}

		_, err := c.multiClusterNamespaceController.Get(clusterName, "", vNamespace)
		if err != nil {
			if errors.IsNotFound(err) {
				// vNamespace not found and pNamespace still exist, we need to delete pNamespace manually
				opts := &metav1.DeleteOptions{
					PropagationPolicy: &constants.DefaultDeletionPolicy,
				}
				if err := c.namespaceClient.Namespaces().Delete(pNamespace.Name, opts); err != nil {
					klog.Errorf("error deleting pNamespace %s in super master: %v", pNamespace.Name, err)
				}
				continue
			}
			klog.Errorf("error getting vNamespace %s from cluster %s cache: %v", vNamespace, clusterName, err)
		}
	}
}

// checkNamespacesOfTenantCluster checks to see if namespaces in specific cluster keeps consistency.
func (c *controller) checkNamespacesOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterNamespaceController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing namespaces from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.Infof("check namespaces consistency in cluster %s", clusterName)
	namespaceList := listObj.(*v1.NamespaceList)
	for i, vNamespace := range namespaceList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vNamespace.Name)
		_, err := c.nsLister.Get(targetNamespace)
		if errors.IsNotFound(err) {
			// pNamespace not found and vNamespace still exists, we need to create pNamespace again
			if err := c.multiClusterNamespaceController.RequeueObject(clusterName, &namespaceList.Items[i], reconciler.AddEvent); err != nil {
				klog.Errorf("error requeue vNamespace %s in cluster %s: %v", vNamespace.Name, clusterName, err)
			}
			continue
		}

		if err != nil {
			klog.Errorf("error getting pNamespace %s from super master cache: %v", targetNamespace, err)
		}
	}
}
