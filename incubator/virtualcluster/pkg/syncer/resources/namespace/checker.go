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
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol/differ"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/featuregate"
	utilconstants "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stopCh, c.nsSynced, c.vcSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Namespace checker")
	}
	c.Patroller.Start(stopCh)
	return nil
}

//  shouldBeGarbageCollected checks if the owner vc object is deleted or not. If so, the namespace should be garbage collected.
func (c *controller) shouldBeGarbageCollected(ns *v1.Namespace) bool {
	vcName := ns.Annotations[constants.LabelVCName]
	vcNamespace := ns.Annotations[constants.LabelVCNamespace]
	if vcName == "" || vcNamespace == "" {
		return false
	}
	vc, err := c.vcLister.VirtualClusters(vcNamespace).Get(vcName)
	if err != nil {
		if errors.IsNotFound(err) {
			// vc does not exist, double check against the apiserver
			if _, apiservererr := c.vcClient.TenancyV1alpha1().VirtualClusters(vcNamespace).Get(vcName, metav1.GetOptions{}); apiservererr != nil {
				if errors.IsNotFound(apiservererr) {
					// vc does not exist in apiserver as well
					return true
				}
			}
		}
	} else {
		// vc exists, check the uid
		if ns.Annotations[constants.LabelVCUID] != string(vc.UID) {
			if v, err := c.vcClient.TenancyV1alpha1().VirtualClusters(vcNamespace).Get(vcName, metav1.GetOptions{}); err == nil {
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

func (c *controller) PatrollerDo() {
	clusterNames := c.MultiClusterController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.V(4).Infof("tenant masters has no clusters, still check pNamespace for gc purpose")
	}

	pList, err := c.nsLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing namespaces from super master informer cache: %v", err)
		return
	}
	pSet := differ.NewDiffSet()
	for _, p := range pList {
		pSet.Insert(differ.ClusterObject{Object: p, Key: p.GetName()})
	}

	blockedClusterSet := sets.NewString()
	vSet := differ.NewDiffSet()
	for _, cluster := range clusterNames {
		listObj, err := c.MultiClusterController.List(cluster)
		if err != nil {
			klog.Errorf("error listing namespaces from cluster %s informer cache: %v", cluster, err)
			blockedClusterSet.Insert(cluster)
			continue
		}
		vList := listObj.(*v1.NamespaceList)
		for i := range vList.Items {
			if featuregate.DefaultFeatureGate.Enabled(featuregate.SuperClusterPooling) {
				if err := mc.IsNamespaceScheduledToCluster(&vList.Items[i], utilconstants.SuperClusterID); err != nil {
					klog.V(4).Infof("skip ns object which is not belongs to this super cluster: %v", err)
					continue
				}
			}
			vSet.Insert(differ.ClusterObject{
				Object:       &vList.Items[i],
				OwnerCluster: cluster,
				Key:          conversion.ToSuperMasterNamespace(cluster, vList.Items[i].GetName()),
			})
		}
	}

	d := differ.HandlerFuncs{}
	d.AddFunc = func(vObj differ.ClusterObject) {
		if err := c.MultiClusterController.RequeueObject(vObj.OwnerCluster, vObj.Object); err != nil {
			klog.Errorf("error requeue vNamespace %v in cluster %s: %v", vObj.GetName(), vObj.GetOwnerCluster(), err)
		} else {
			metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantNamespaces").Inc()
		}
	}
	d.UpdateFunc = func(vObj, pObj differ.ClusterObject) {
		v := vObj.Object.(*v1.Namespace)
		p := pObj.Object.(*v1.Namespace)

		// if vc object is deleted, we should reach here
		if c.shouldBeGarbageCollected(p) || p.Annotations[constants.LabelUID] != string(v.UID) {
			c.deleteNamespace(p)
		}
	}
	d.DeleteFunc = func(pObj differ.ClusterObject) {
		p := pObj.Object.(*v1.Namespace)
		if p.Annotations[constants.LabelVCRootNS] == "true" {
			if !c.shouldBeGarbageCollected(p) {
				return
			}
		}
		c.deleteNamespace(p)
	}

	vSet.Difference(pSet, differ.FilteringHandler{
		Handler: d,
		FilterFunc: func(obj differ.ClusterObject) bool {
			if obj.OwnerCluster == "" && obj.GetAnnotations()[constants.LabelVCRootNS] == "true" {
				return true
			}
			return differ.DefaultDifferFilter(blockedClusterSet)(obj)
		},
	})
}

func (c *controller) deleteNamespace(ns *v1.Namespace) {
	deleteOptions := &metav1.DeleteOptions{}
	deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(ns.GetUID()))
	if err := c.namespaceClient.Namespaces().Delete(context.TODO(), ns.GetName(), *deleteOptions); err != nil {
		klog.Errorf("error deleting pNamespace %s in super master: %v", ns.GetName(), err)
	} else {
		metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterNamespaces").Inc()
	}
}
