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
)

func (c *controller) StartPeriodChecker(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.pvSynced, c.pvcSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service checker")
	}

	wait.Until(c.checkPVs, c.periodCheckerPeriod, stopCh)
	return nil
}

// checkPVs check if persistent volumes keep consistency between super master and tenant masters.
func (c *controller) checkPVs() {
	clusterNames := c.multiClusterPersistentVolumeController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}
	wg := sync.WaitGroup{}

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkPersistentVolumeOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pvList, err := c.pvLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing pv from super master informer cache: %v", err)
		return
	}

	for _, pPV := range pvList {
		if pPV.Spec.ClaimRef == nil {
			continue
		}
		pPVC, err := c.pvcLister.PersistentVolumeClaims(pPV.Spec.ClaimRef.Namespace).Get(pPV.Spec.ClaimRef.Name)
		if err != nil {
			if !errors.IsNotFound(err) {
				klog.Errorf("fail to get pPVC %s/%s in super master :%v", pPVC.Namespace, pPVC.Name, err)
			}
			continue
		}
		clusterName, vNamespace := conversion.GetVirtualOwner(pPVC)
		if clusterName == "" {
			// Bound PVC does not belong to any tenant.
			continue
		}
		vPVObj, err := c.multiClusterPersistentVolumeController.Get(clusterName, "", pPV.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				c.queue.Add(pPV.Name)
			}
			klog.Errorf("fail to get pv %s from cluster %s: %v", pPV.Name, clusterName, err)
		} else {
			// Double check if the vPV is bound to the correct PVC.
			vPV := vPVObj.(*v1.PersistentVolume)
			if vPV.Spec.ClaimRef == nil || vPV.Spec.ClaimRef.Name != pPVC.Name || vPV.Spec.ClaimRef.Namespace != vNamespace {
				klog.Errorf("vPV %v from cluster %s is not bound to the correct pvc", vPV, clusterName)
			}
		}
	}
}

func (c *controller) checkPersistentVolumeOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterPersistentVolumeController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing pv from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.Infof("check pv consistency in cluster %s", clusterName)
	pvList := listObj.(*v1.PersistentVolumeList)
	for _, vPV := range pvList.Items {
		pPV, err := c.pvLister.Get(vPV.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				// We disallow tenant to create its own PVs, hence we delete them.
				// If this pv is still bound to pvc, print an error msg. Normally, the deleted PV should be in Relased phase.
				if vPV.Spec.ClaimRef != nil && vPV.Status.Phase == "Bound" {
					klog.Errorf("Removed pv %s in cluster %s is bound to a pvc", vPV.Name, clusterName)
				}
				tenantClient, err := c.multiClusterPersistentVolumeController.GetClusterClient(clusterName)
				if err != nil {
					klog.Errorf("error getting cluster %s clientset: %v", clusterName, err)
					continue
				}
				opts := &metav1.DeleteOptions{
					PropagationPolicy: &constants.DefaultDeletionPolicy,
				}
				if err := tenantClient.CoreV1().PersistentVolumes().Delete(vPV.Name, opts); err != nil {
					klog.Errorf("error deleting pv %v in cluster %s: %v", vPV.Name, clusterName, err)
				}
			} else {
				klog.Errorf("failed to get pPV %s from super master cache: %v", vPV.Name, err)
			}
			continue
		}
		updatedPVSpec := conversion.Equality(nil).CheckPVSpecEquality(&pPV.Spec, &vPV.Spec)
		if updatedPVSpec != nil {
			klog.Warningf("spec of pv %v diff in super&tenant master %s", vPV.Name, clusterName)
		}
	}
}
