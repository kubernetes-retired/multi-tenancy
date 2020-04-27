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
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
)

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stopCh, c.nsSynced, c.vcSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Namespace checker")
	}
	c.namespacePatroller.Start(stopCh)
	return nil
}

//  shouldBeGabageCollected checks if the owner vc object is deleted or not. If so, the namespace should be garbage collected.
func (c *controller) shouldBeGabageCollected(ns *v1.Namespace) bool {
	vcName := ns.Annotations[constants.LabelVCName]
	vcNamespace := ns.Annotations[constants.LabelVCNamespace]
	if vcName == "" || vcNamespace == "" {
		return false
	}
	vc, err := c.vcLister.Virtualclusters(vcNamespace).Get(vcName)
	if err != nil {
		if errors.IsNotFound(err) {
			// vc does not exist, double check against the apiserver
			if _, apiservererr := c.vcClient.TenancyV1alpha1().Virtualclusters(vcNamespace).Get(vcName, metav1.GetOptions{}); apiservererr != nil {
				if errors.IsNotFound(apiservererr) {
					// vc does not exist in apiserver as well
					return true
				}
			}
		}
	} else {
		// vc exists, check the uid
		if ns.Annotations[constants.LabelVCUID] != string(vc.UID) {
			if v, err := c.vcClient.TenancyV1alpha1().Virtualclusters(vcNamespace).Get(vcName, metav1.GetOptions{}); err == nil {
				if ns.Annotations[constants.LabelVCUID] != string(v.UID) {
					// uid is indeed different
					return true
				}
			}
		}
		klog.V(4).Infof("pNamespace %s's owner vc exists.", ns.Name)
	}
	return false
}

// PatrollerDo checks to see if namespaces in super master informer cache and tenant master
// keep consistency.
func (c *controller) PatrollerDo() {
	defer metrics.RecordCheckerScanDuration("namespace", time.Now())
	clusterNames := c.multiClusterNamespaceController.GetClusterNames()
	if len(clusterNames) != 0 {
		wg := sync.WaitGroup{}
		for _, clusterName := range clusterNames {
			wg.Add(1)
			go func(clusterName string) {
				defer wg.Done()
				c.checkNamespacesOfTenantCluster(clusterName)
			}(clusterName)
		}
		wg.Wait()
	} else {
		klog.Infof("tenant masters has no clusters, still check pNamespace for gc purpose.")
	}

	pNamespaces, err := c.nsLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing namespaces from super master informer cache: %v", err)
		return
	}

	for _, pNamespace := range pNamespaces {
		shouldDelete := false
		if pNamespace.Annotations[constants.LabelVCRootNS] == "true" {
			if c.shouldBeGabageCollected(pNamespace) {
				shouldDelete = true
			}
		} else {
			clusterName, vNamespace := conversion.GetVirtualOwner(pNamespace)
			if len(clusterName) == 0 || len(vNamespace) == 0 {
				continue
			}
			vNamespaceObj, err := c.multiClusterNamespaceController.Get(clusterName, "", vNamespace)
			if err != nil {
				// if vc object is deleted, we should reach here
				if errors.IsNotFound(err) || c.shouldBeGabageCollected(pNamespace) {
					shouldDelete = true
				}
			} else {
				vNs := vNamespaceObj.(*v1.Namespace)
				if pNamespace.Annotations[constants.LabelUID] != string(vNs.UID) {
					shouldDelete = true
					klog.Warningf("Found pNamespace %s delegated UID is different from tenant object.", pNamespace.Name)
				}
			}
		}

		if shouldDelete {
			opts := &metav1.DeleteOptions{
				PropagationPolicy: &constants.DefaultDeletionPolicy,
				Preconditions:     metav1.NewUIDPreconditions(string(pNamespace.UID)),
			}
			if err := c.namespaceClient.Namespaces().Delete(pNamespace.Name, opts); err != nil {
				klog.Errorf("error deleting pNamespace %s in super master: %v", pNamespace.Name, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("numDeletedOrphanSuperMasterNamespaces").Inc()
			}
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
	klog.V(4).Infof("check namespaces consistency in cluster %s", clusterName)
	namespaceList := listObj.(*v1.NamespaceList)
	for i, vNamespace := range namespaceList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vNamespace.Name)
		pNamespace, err := c.nsLister.Get(targetNamespace)
		if errors.IsNotFound(err) {
			// pNamespace not found and vNamespace still exists, we need to create pNamespace again
			if err := c.multiClusterNamespaceController.RequeueObject(clusterName, &namespaceList.Items[i]); err != nil {
				klog.Errorf("error requeue vNamespace %s in cluster %s: %v", vNamespace.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("numRequeuedTenantNamespaces").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("error getting pNamespace %s from super master cache: %v", targetNamespace, err)
		}

		if pNamespace.Annotations[constants.LabelUID] != string(vNamespace.UID) {
			klog.Errorf("Found pNamespace %s delegated UID is different from tenant object.", targetNamespace)
		}
	}
}
