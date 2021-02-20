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

package persistentvolume

import (
	"context"
	"fmt"
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
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol/differ"
)

var numClaimMissMatchedPVs uint64
var numSpecMissMatchedPVs uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.pvSynced, c.pvcSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service checker")
	}
	c.Patroller.Start(stopCh)
	return nil
}

// PatrollerDo check if persistent volumes keep consistency between super master and tenant masters.
func (c *controller) PatrollerDo() {
	clusterNames := c.MultiClusterController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	numClaimMissMatchedPVs = 0
	numSpecMissMatchedPVs = 0

	pList, err := c.pvLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing pv from super master informer cache: %v", err)
		return
	}
	pSet := differ.NewDiffSet()
	for _, p := range pList {
		pSet.Insert(differ.ClusterObject{Object: p, Key: p.GetName()})
	}

	vSet := differ.NewDiffSet()
	for _, cluster := range clusterNames {
		listObj, err := c.MultiClusterController.List(cluster)
		if err != nil {
			klog.Errorf("error listing pv from cluster %s informer cache: %v", cluster, err)
			continue
		}
		vList := listObj.(*v1.PersistentVolumeList)
		for i := range vList.Items {
			vSet.Insert(differ.ClusterObject{
				Object:       &vList.Items[i],
				OwnerCluster: cluster,
				Key:          vList.Items[i].GetName(),
			})
		}
	}

	d := differ.HandlerFuncs{}
	d.AddFunc = func(pObj differ.ClusterObject) {
		c.UpwardController.AddToQueue(pObj.GetName())
		metrics.CheckerRemedyStats.WithLabelValues("RequeuedSuperMasterPVs").Inc()
	}
	d.UpdateFunc = func(pObj, vObj differ.ClusterObject) {
		pPV := pObj.Object.(*v1.PersistentVolume)
		vPV := vObj.Object.(*v1.PersistentVolume)

		pPVC, err := c.pvcLister.PersistentVolumeClaims(pPV.Spec.ClaimRef.Namespace).Get(pPV.Spec.ClaimRef.Name)
		if err != nil {
			return
		}
		clusterName, vNamespace := conversion.GetVirtualOwner(pPVC)
		if clusterName == "" {
			// Bound PVC does not belong to any tenant.
			return
		}

		// Double check if the vPV is bound to the correct PVC.
		if vPV.Spec.ClaimRef == nil || vPV.Spec.ClaimRef.Name != pPVC.Name || vPV.Spec.ClaimRef.Namespace != vNamespace {
			klog.Errorf("vPV %v from cluster %s is not bound to the correct pvc", vPV.GetName(), clusterName)
			numClaimMissMatchedPVs++
		}

		if vPV.Annotations[constants.LabelUID] != string(pPV.UID) {
			d.OnDelete(vObj)
			return
		}

		updatedPVSpec := conversion.Equality(c.Config, nil).CheckPVSpecEquality(&pPV.Spec, &vPV.Spec)
		if updatedPVSpec != nil {
			atomic.AddUint64(&numSpecMissMatchedPVs, 1)
			klog.Warningf("spec of pv %v diff in super&tenant master %s", vPV.Name, clusterName)
			if boundPersistentVolume(pPV) {
				c.enqueuePersistentVolume(pPV)
			}
		}
	}
	d.DeleteFunc = func(vObj differ.ClusterObject) {
		vPV := vObj.Object.(*v1.PersistentVolume)

		// We delete any PV created by tenant.
		// If the pv is still bound to pvc, print an error msg. Normally, the deleted PV should be in Relased phase.
		if vPV.Spec.ClaimRef != nil && vPV.Status.Phase == "Bound" {
			klog.Errorf("Removed pv %s in cluster %s is bound to a pvc", vPV.Name, vObj.GetOwnerCluster())
		}

		tenantClient, err := c.MultiClusterController.GetClusterClient(vObj.GetOwnerCluster())
		if err != nil {
			klog.Errorf("error getting cluster %s clientset: %v", vObj.GetOwnerCluster(), err)
			return
		}
		opts := &metav1.DeleteOptions{
			PropagationPolicy: &constants.DefaultDeletionPolicy,
			Preconditions:     metav1.NewUIDPreconditions(string(vPV.UID)),
		}
		if err := tenantClient.CoreV1().PersistentVolumes().Delete(context.TODO(), vPV.Name, *opts); err != nil {
			klog.Errorf("error deleting pv %v in cluster %s: %v", vPV.Name, vObj.GetOwnerCluster(), err)
		} else {
			metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanTenantPVs").Inc()
		}
	}

	pSet.Difference(vSet, differ.FilteringHandler{
		Handler: d,
		FilterFunc: func(obj differ.ClusterObject) bool {
			// if both vObj pObj exists, pObj may not pass the filter.
			// differ will skip this onUpdate.
			// don't worry to delete vObj accidentally.

			if obj.OwnerCluster != "" {
				return true
			}

			pPV := obj.Object.(*v1.PersistentVolume)
			if !boundPersistentVolume(pPV) {
				return false
			}

			pPVC, err := c.pvcLister.PersistentVolumeClaims(pPV.Spec.ClaimRef.Namespace).Get(pPV.Spec.ClaimRef.Name)
			if err != nil {
				if !errors.IsNotFound(err) {
					klog.Errorf("fail to get pPVC %s/%s in super master :%v", pPVC.Namespace, pPVC.Name, err)
				}
				return false
			}
			clusterName, _ := conversion.GetVirtualOwner(pPVC)
			if clusterName == "" {
				// Bound PVC does not belong to any tenant.
				return false
			}
			return true
		},
	})

	metrics.CheckerMissMatchStats.WithLabelValues("ClaimMissMatchedPVs").Set(float64(numClaimMissMatchedPVs))
	metrics.CheckerMissMatchStats.WithLabelValues("SpecMissMatchedPVs").Set(float64(numSpecMissMatchedPVs))

}
