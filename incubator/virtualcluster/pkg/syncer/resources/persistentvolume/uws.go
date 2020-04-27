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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.pvSynced, c.pvcSynced) {
		return fmt.Errorf("failed to wait for caches to sync persistentvolume")
	}
	return c.upwardPersistentVolumeController.Start(stopCh)
}

func (c *controller) BackPopulate(key string) error {
	pPV, err := c.pvLister.Get(key)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if pPV.Spec.ClaimRef == nil {
		return nil
	}

	pPVC, err := c.pvcLister.PersistentVolumeClaims(pPV.Spec.ClaimRef.Namespace).Get(pPV.Spec.ClaimRef.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			// Bound PVC is gone, we cannot find the tenant who owns the pv. Checker will fix any possible race.
			return nil
		}
		return err
	}

	clusterName, vNamespace := conversion.GetVirtualOwner(pPVC)
	if clusterName == "" {
		// Bound PVC does not belong to any tenant.
		return nil
	}

	tenantClient, err := c.multiClusterPersistentVolumeController.GetClusterClient(clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}

	vPVObj, err := c.multiClusterPersistentVolumeController.Get(clusterName, "", key)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create a new pv with bound claim in tenant master
			vPVC, err := tenantClient.CoreV1().PersistentVolumeClaims(vNamespace).Get(pPVC.Name, metav1.GetOptions{})
			if err != nil {
				// If corresponding pvc does not exist in tenant, we'll let checker fix any possible race.
				klog.Errorf("Cannot find the bound pvc %s/%s in tenant cluster %s for pv %v", vNamespace, pPVC.Name, clusterName, pPV)
				return nil
			}

			vPV := conversion.BuildVirtualPersistentVolume(clusterName, pPV, vPVC)
			_, err = tenantClient.CoreV1().PersistentVolumes().Create(vPV)
			if err != nil {
				return err
			}
			return nil
		}
		return err
	}

	vPV := vPVObj.(*v1.PersistentVolume)
	if vPV.Annotations[constants.LabelUID] != string(pPV.UID) {
		return fmt.Errorf("vPV %s in cluster %s delegated UID is different from pPV.", vPV.Name, clusterName)
	}

	// We only update PV.Spec, PV.Status is managed by tenant/super pv binder controller independently.
	updatedPVSpec := conversion.Equality(c.config, nil).CheckPVSpecEquality(&pPV.Spec, &vPV.Spec)
	if updatedPVSpec != nil {
		newPV := vPV.DeepCopy()
		newPV.Spec = *updatedPVSpec
		_, err := tenantClient.CoreV1().PersistentVolumes().Update(newPV)
		if err != nil {
			return err
		}
	}
	return nil
}
