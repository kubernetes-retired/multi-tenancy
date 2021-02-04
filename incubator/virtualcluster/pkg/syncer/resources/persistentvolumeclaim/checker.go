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

package persistentvolumeclaim

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

var numMissMatchedPVCs uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.pvcSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service checker")
	}
	c.Patroller.Start(stopCh)
	return nil
}

// PatrollerDo check if persistent volume claims keep consistency between super
// master and tenant masters.
func (c *controller) PatrollerDo() {
	clusterNames := c.MultiClusterController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	wg := sync.WaitGroup{}
	numMissMatchedPVCs = 0

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkPVCOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pPVCs, err := c.pvcLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing PVCs from super master informer cache: %v", err)
		return
	}

	for _, pPVC := range pPVCs {
		clusterName, vNamespace := conversion.GetVirtualOwner(pPVC)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}
		shouldDelete := false
		vPVCObj, err := c.MultiClusterController.Get(clusterName, vNamespace, pPVC.Name)
		if errors.IsNotFound(err) {
			shouldDelete = true
		}
		if err == nil {
			vPVC := vPVCObj.(*v1.PersistentVolumeClaim)
			if pPVC.Annotations[constants.LabelUID] != string(vPVC.UID) {
				shouldDelete = true
				klog.Warningf("Found pPVC %s/%s delegated UID is different from tenant object.", pPVC.Namespace, pPVC.Name)
			}
		}
		if shouldDelete {
			deleteOptions := metav1.NewPreconditionDeleteOptions(string(pPVC.UID))
			if err = c.pvcClient.PersistentVolumeClaims(pPVC.Namespace).Delete(context.TODO(), pPVC.Name, *deleteOptions); err != nil {
				klog.Errorf("error deleting pPVC %s/%s in super master: %v", pPVC.Namespace, pPVC.Name, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterPVCs").Inc()
			}
		}
	}

	metrics.CheckerMissMatchStats.WithLabelValues("MissMatchedPVCs").Set(float64(numMissMatchedPVCs))
}

func (c *controller) checkPVCOfTenantCluster(clusterName string) {
	listObj, err := c.MultiClusterController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing PVCs from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.V(4).Infof("check PVCs consistency in cluster %s", clusterName)
	pvcList := listObj.(*v1.PersistentVolumeClaimList)
	for i, vPVC := range pvcList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vPVC.Namespace)
		pPVC, err := c.pvcLister.PersistentVolumeClaims(targetNamespace).Get(vPVC.Name)
		if errors.IsNotFound(err) {
			if err := c.MultiClusterController.RequeueObject(clusterName, &pvcList.Items[i]); err != nil {
				klog.Errorf("error requeue vPVC %v/%v in cluster %s: %v", vPVC.Namespace, vPVC.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantPVCs").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("failed to get pPVC %s/%s from super master cache: %v", targetNamespace, vPVC.Name, err)
			continue
		}

		if pPVC.Annotations[constants.LabelUID] != string(vPVC.UID) {
			klog.Warningf("Found pPVC %s/%s delegated UID is different from tenant object.", pPVC.Namespace, pPVC.Name)
			continue
		}

		vc, err := util.GetVirtualClusterObject(c.MultiClusterController, clusterName)
		if err != nil {
			klog.Errorf("fail to get cluster spec : %s", clusterName)
			continue
		}
		updatedPVC := conversion.Equality(c.Config, vc).CheckPVCEquality(pPVC, &vPVC)
		if updatedPVC != nil {
			atomic.AddUint64(&numMissMatchedPVCs, 1)
			klog.Warningf("spec of pvc %v/%v diff in super&tenant master", vPVC.Namespace, vPVC.Name)
		}
	}
}
