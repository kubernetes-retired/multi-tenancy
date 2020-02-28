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

package event

import (
	"fmt"
	"time"

	"k8s.io/api/core/v1"
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

	klog.Infof("starting event upward syncer")

	if !cache.WaitForCacheSync(stopCh, c.eventSynced, c.nsSynced) {
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
	obj, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(obj)

	req, ok := obj.(reconciler.UwsRequest)
	if !ok {
		c.queue.Forget(obj)
		return true
	}

	klog.V(4).Infof("back populate event %+v", req.Key)
	err := c.backPopulate(req.Key)
	if err == nil {
		c.queue.Forget(obj)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing event %v (will retry): %v", req.Key, err))
	if req.FirstFailureTime == nil {
		now := metav1.Now()
		req.FirstFailureTime = &now
	} else {
		if metav1.Now().After(req.FirstFailureTime.Add(constants.DefaultUwsRetryTimePeriod)) {
			klog.Warningf("Event uws request is dropped due to timeout: %v", req)
			c.queue.Forget(obj)
			return true
		}
	}
	c.queue.AddRateLimited(obj)
	return true
}

func (c *controller) backPopulate(key string) error {
	pNamespace, pName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key %v: %v", key, err))
		return nil
	}

	pEvent, err := c.eventLister.Events(pNamespace).Get(pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("could not find pEvent %s/%s in controller cache: %v", pNamespace, pName, err)
	}

	clusterName, tenantNS, err := conversion.GetVirtualNamespace(c.nsLister, pNamespace)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("could not find ns %s in controller cache: %v", pNamespace, err)
	}
	if clusterName == "" || tenantNS == "" {
		klog.Infof("drop event %s/%s which is not belongs to any tenant", pNamespace, pName)
		return nil
	}

	tenantClient, err := c.multiClusterEventController.GetClusterClient(clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}

	vPodObj, err := c.multiClusterEventController.GetByObjectType(clusterName, tenantNS, pEvent.InvolvedObject.Name, &v1.Pod{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("back populate event: failed to find pod %s/%s in cluster %s", tenantNS, pEvent.InvolvedObject.Name, clusterName)
			return nil
		}
		return err
	}
	vPod := vPodObj.(*v1.Pod)
	vEvent := conversion.BuildVirtualPodEvent(clusterName, pEvent, vPod)
	_, err = c.multiClusterEventController.Get(clusterName, tenantNS, vEvent.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = tenantClient.CoreV1().Events(tenantNS).Create(vEvent)
			return err
		}
		return err
	}
	return nil
}
