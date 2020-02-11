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

package storageclass

import (
	"fmt"
	"time"

	v1 "k8s.io/api/storage/v1"
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

	klog.Infof("starting storageclass upward syncer")

	if !cache.WaitForCacheSync(stopCh, c.storageclassSynced) {
		return fmt.Errorf("failed to wait for caches to sync storageclass")
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
	req, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(req)

	klog.Infof("back populate storageclass %+v", req)
	err := c.backPopulate(req.(scReconcileRequest))
	if err == nil {
		c.queue.Forget(req)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing storageclass %v (will retry): %v", req, err))
	c.queue.AddRateLimited(req)
	return true
}

func (c *controller) backPopulate(req scReconcileRequest) error {
	op := reconciler.AddEvent
	pStorageClass, err := c.storageclassLister.Get(req.key)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		op = reconciler.DeleteEvent
	}

	tenantClient, err := c.multiClusterStorageClassController.GetClusterClient(req.clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", req.clusterName, err)
	}

	vStorageClassObj, err := c.multiClusterStorageClassController.Get(req.clusterName, "", req.key)
	if err != nil {
		if errors.IsNotFound(err) {
			if op == reconciler.AddEvent {
				// Available in super, hence create a new in tenant master
				vStorageClass := conversion.BuildVirtualStorageClass(req.clusterName, pStorageClass)
				_, err := tenantClient.StorageV1().StorageClasses().Create(vStorageClass)
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
		err := tenantClient.StorageV1().StorageClasses().Delete(req.key, opts)
		if err != nil {
			return err
		}
	} else {
		updatedStorageClass := conversion.Equality(nil).CheckStorageClassEquality(pStorageClass, vStorageClassObj.(*v1.StorageClass))
		if updatedStorageClass != nil {
			_, err := tenantClient.StorageV1().StorageClasses().Update(updatedStorageClass)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
