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

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/node"
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

	klog.V(4).Infof("back populate pod %+v", req.Key)
	err := c.backPopulate(req.Key)
	if err == nil {
		c.queue.Forget(obj)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing pod %v (will retry): %v", req.Key, err))
	if req.FirstFailureTime == nil {
		now := metav1.Now()
		req.FirstFailureTime = &now
	} else {
		if metav1.Now().After(req.FirstFailureTime.Add(constants.DefaultUwsRetryTimePeriod)) {
			klog.Warningf("Pod uws request is dropped due to timeout: %v", req)
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

	defer metrics.RecordUWSOperationDuration("pod", time.Now())
	pPod, err := c.podLister.Pods(pNamespace).Get(pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	clusterName, vNamespace := conversion.GetVirtualOwner(pPod)
	if clusterName == "" || vNamespace == "" {
		klog.Infof("drop pod %s/%s which is not belongs to any tenant", pNamespace, pName)
		return nil
	}

	vPodObj, err := c.multiClusterPodController.Get(clusterName, vNamespace, pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("could not find pPod %s/%s's vPod in controller cache %v", vNamespace, pName, err)
	}
	vPod := vPodObj.(*v1.Pod)
	if pPod.Annotations[constants.LabelUID] != string(vPod.UID) {
		return fmt.Errorf("BackPopulated pPod %s/%s delegated UID is different from updated object.", pPod.Namespace, pPod.Name)
	}

	tenantClient, err := c.multiClusterPodController.GetClusterClient(clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}

	// If tenant Pod has not been assigned, bind to virtual Node.
	if vPod.Spec.NodeName == "" {
		n, err := c.client.Nodes().Get(pPod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get node %s from super master: %v", pPod.Spec.NodeName, err)
		}
		// We need to handle the race with vNodeGC thread here.
		if err = func() error {
			c.Lock()
			defer c.Unlock()
			if !c.removeQuiescingNodeFromClusterVNodeGCMap(clusterName, pPod.Spec.NodeName) {
				return fmt.Errorf("The bind target vNode %s is being GCed in cluster %s, retry", pPod.Spec.NodeName, clusterName)
			}
			return nil
		}(); err != nil {
			return err
		}

		if _, err := c.multiClusterPodController.GetByObjectType(clusterName, vNamespace, n.GetName(), &v1.Node{}); err != nil {
			// check if target node has already registered on the vc
			// before creating
			if !errors.IsNotFound(err) {
				return err
			}
			_, err = tenantClient.CoreV1().Nodes().Create(node.NewVirtualNode(n))
			if err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create virtual node %s in cluster %s with err: %v", pPod.Spec.NodeName, clusterName, err)
			}
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
		// virtual pod has been updated, refetch the latest version
		if vPod, err = tenantClient.CoreV1().Pods(vPod.Namespace).Get(vPod.Name, metav1.GetOptions{}); err != nil {
			return fmt.Errorf("failed to retrieve vPod %s/%s from cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
		}
	} else {
		// Check if the vNode exists in Tenant master.
		if _, err := tenantClient.CoreV1().Nodes().Get(vPod.Spec.NodeName, metav1.GetOptions{}); err != nil {
			if !errors.IsNotFound(err) {
				// We have consistency issue here, do not fix for now. TODO: add to metrics
			}
			return fmt.Errorf("failed to check vNode %s of vPod %s in cluster %s: %v ", vPod.Spec.NodeName, vPod.Name, clusterName, err)
		}
	}

	spec, err := c.multiClusterPodController.GetSpec(clusterName)
	if err != nil {
		return err
	}

	var newPod *v1.Pod
	updatedMeta := conversion.Equality(spec).CheckUWObjectMetaEquality(&pPod.ObjectMeta, &vPod.ObjectMeta)
	if updatedMeta != nil {
		newPod = vPod.DeepCopy()
		newPod.ObjectMeta = *updatedMeta
		if _, err = tenantClient.CoreV1().Pods(vPod.Namespace).Update(newPod); err != nil {
			return fmt.Errorf("failed to back populate pod %s/%s meta update for cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
		}
	}

	if !equality.Semantic.DeepEqual(vPod.Status, pPod.Status) {
		if newPod == nil {
			newPod = vPod.DeepCopy()
		} else {
			// Pod has been updated, let us fetch the latest version.
			if newPod, err = tenantClient.CoreV1().Pods(vPod.Namespace).Get(vPod.Name, metav1.GetOptions{}); err != nil {
				return fmt.Errorf("failed to retrieve vPod %s/%s from cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
			}
		}
		newPod.Status = pPod.Status
		if _, err = tenantClient.CoreV1().Pods(vPod.Namespace).UpdateStatus(newPod); err != nil {
			return fmt.Errorf("failed to back populate pod %s/%s status update for cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
		}
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
			if err = tenantClient.CoreV1().Pods(vPod.Namespace).Delete(vPod.Name, deleteOptions); err != nil {
				return err
			}
			if vPod.Spec.NodeName != "" && isPodScheduled(vPod) {
				c.updateClusterVNodePodMap(clusterName, vPod.Spec.NodeName, string(vPod.UID), reconciler.DeleteEvent)
			}
		}
	}

	return nil
}
