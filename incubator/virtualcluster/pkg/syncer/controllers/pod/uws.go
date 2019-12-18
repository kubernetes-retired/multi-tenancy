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

package pod

import (
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controllers/node"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.queue.ShutDown()

	klog.Infof("starting pod upward syncer")

	if !cache.WaitForCacheSync(stopCh, c.podSynced, c.serviceSynced) {
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

// podQueueKey holds information we need to sync a pod.
// It contains enough information to look up the cluster resource.
type podQueueKey struct {
	// clusterName is the cluster this pod belongs to.
	clusterName string
	// vNamespace is the namespace this pod on tenant namespaces.
	vNamespace string
	// namespace is the namespace this pod on super master namespaces.
	namespace string
	// name is the pod name.
	name string
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

	klog.Infof("back populate pod %+v", key)
	err := c.backPopulate(key)
	if err == nil {
		c.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing pod %v (will retry): %v", key, err))
	c.queue.AddRateLimited(key)
	return true
}

func (c *controller) backPopulate(key interface{}) error {
	podInfo, ok := key.(podQueueKey)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	tenantCluster := c.multiClusterPodController.GetCluster(podInfo.clusterName)
	if tenantCluster == nil {
		return fmt.Errorf("cluster %s not found", podInfo.clusterName)
	}
	tenantClient, err := tenantCluster.GetClient()
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", podInfo.clusterName, err)
	}

	pPod, err := c.podLister.Pods(podInfo.namespace).Get(podInfo.name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	vPodObj, err := c.multiClusterPodController.Get(podInfo.clusterName, podInfo.vNamespace, podInfo.name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("could not find pPod %s/%s pPod in controller cache %v", podInfo.vNamespace, pPod.Name, err)
	}
	vPod := vPodObj.(*v1.Pod)

	// first check whether tenant pod has assigned.
	if vPod.Spec.NodeName != pPod.Spec.NodeName {
		n, err := c.client.Nodes().Get(pPod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get node %s from super master: %v", pPod.Spec.NodeName, err)
		}

		_, err = tenantClient.CoreV1().Nodes().Create(node.NewVirtualNode(n))
		if errors.IsAlreadyExists(err) {
			klog.Warningf("virtual node %s already exists", vPod.Spec.NodeName)
		}

		err = tenantClient.CoreV1().Pods(vPod.Namespace).Bind(&v1.Binding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vPod.Name,
				Namespace: vPod.Namespace,
			},
			Target: v1.ObjectReference{
				Kind:       "Node",
				Name:       pPod.Spec.NodeName,
				APIVersion: "v1",
			},
		})
		if err != nil {
			return fmt.Errorf("failed to bind vPod %s/%s to node %s %v", vPod.Namespace, vPod.Name, pPod.Spec.NodeName, err)
		}
		// virtual pod has been updated, return and waiting for next loop.
		c.queue.AddAfter(key, 1*time.Second)
		return nil
	}

	if !equality.Semantic.DeepEqual(vPod.Status, pPod.Status) {
		newPod := vPod.DeepCopy()
		newPod.Status = pPod.Status
		if _, err = tenantClient.CoreV1().Pods(vPod.Namespace).UpdateStatus(newPod); err != nil {
			return fmt.Errorf("failed to back populate pod %s/%s status update for cluster %s: %v", vPod.Namespace, vPod.Name, podInfo.clusterName, err)
		}
		c.queue.Add(key)
		return nil
	}

	// pPod is under deletion.
	if pPod.DeletionTimestamp != nil {
		if vPod.DeletionTimestamp == nil {
			klog.V(4).Infof("pPod %s/%s is under deletion accidentally", pPod.Namespace, pPod.Name)
			// waiting for periodic check to recreate a pPod on super master.
			return nil
		}
		if *vPod.DeletionGracePeriodSeconds != *pPod.DeletionGracePeriodSeconds {
			klog.V(4).Infof("delete virtual pPod %s/%s with grace period seconds %v", vPod.Namespace, vPod.Name, *pPod.DeletionGracePeriodSeconds)
			deleteOptions := metav1.NewDeleteOptions(*pPod.DeletionGracePeriodSeconds)
			deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(vPod.UID))
			return tenantClient.CoreV1().Pods(vPod.Namespace).Delete(vPod.Name, deleteOptions)
		}
	}

	return nil
}
