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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("starting persistentvolume upward syncer")

	if !cache.WaitForCacheSync(stopCh, c.pvSynced, c.pvcSynced) {
		return fmt.Errorf("failed to wait for caches to sync persistentvolume")
	}

	klog.V(5).Infof("starting workers")
	for i := 0; i < c.workers; i++ {
		go wait.Until(c.run, 1*time.Second, stopCh)
	}
	<-stopCh
	klog.V(1).Infof("shutting down")

	return nil
}

// run runs a run thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (c *controller) run() {
	for c.processNextWorkItem() {
	}
}

func (c *controller) processNextWorkItem() bool {
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

	klog.Infof("back populate persistentvolume %+v", key)
	err := c.backPopulate(key.(string))
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing persistentvolume %v (will retry): %v", key, err))
	c.queue.AddRateLimited(key)
	return true
}

func (c *controller) backPopulate(key string) error {
	op := reconciler.AddEvent
	pPV, err := c.pvLister.Get(key)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		op = reconciler.DeleteEvent
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
			if op == reconciler.AddEvent {
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
			}
			return nil
		}
		return err
	}

	if op == reconciler.DeleteEvent {
		opts := &metav1.DeleteOptions{
			PropagationPolicy: &constants.DefaultDeletionPolicy,
		}
		err := tenantClient.CoreV1().PersistentVolumes().Delete(key, opts)
		if err != nil {
			return err
		}
	} else {
		vPV := vPVObj.(*v1.PersistentVolume)
		// We only update PV.Spec, PV.Status is managed by tenant/super pv binder controller independently.
		updatedPVSpec := conversion.Equality(nil).CheckPVSpecEquality(&pPV.Spec, &vPV.Spec)
		if updatedPVSpec != nil {
			newPV := vPV.DeepCopy()
			newPV.Spec = *updatedPVSpec
			_, err := tenantClient.CoreV1().PersistentVolumes().Update(newPV)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
