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

package service

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("starting service upward syncer")

	if !cache.WaitForCacheSync(stopCh, c.serviceSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
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

	klog.Infof("back populate service %+v", key)
	err := c.backPopulate(key.(string))
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing pod %v (will retry): %v", key, err))
	c.queue.AddRateLimited(key)
	return true
}

func (c *controller) backPopulate(key string) error {
	pNamespace, pName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key %v: %v", key, err))
		return nil
	}

	pService, err := c.serviceLister.Services(pNamespace).Get(pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	clusterName, vNamespace := conversion.GetVirtualOwner(pService)
	if clusterName == "" || vNamespace == "" {
		klog.Infof("drop service %s/%s which is not belongs to any tenant", pNamespace, pName)
		return nil
	}

	vServiceObj, err := c.multiClusterServiceController.Get(clusterName, vNamespace, pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("could not find pService %s/%s's vService in controller cache %v", vNamespace, pName, err)
	}
	vService := vServiceObj.(*v1.Service)

	tenantClient, err := c.multiClusterServiceController.GetClusterClient(clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}

	if !equality.Semantic.DeepEqual(vService.Status, pService.Status) {
		newService := vService.DeepCopy()
		newService.Status = pService.Status
		if _, err = tenantClient.CoreV1().Services(vService.Namespace).UpdateStatus(newService); err != nil {
			return fmt.Errorf("failed to back populate service %s/%s status update for cluster %s: %v", vService.Namespace, vService.Name, clusterName, err)
		}
	}
	return nil
}
