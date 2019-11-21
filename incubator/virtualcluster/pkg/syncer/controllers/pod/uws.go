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
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controllers/node"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
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
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(key)

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
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	pod, err := c.podLister.Pods(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	clusterName, vNamespace := conversion.GetOwner(pod)
	if len(clusterName) == 0 {
		return nil
	}

	klog.Infof("back populate pod %s/%s in cluster %s", vNamespace, pod.Name, clusterName)
	vPodObj, err := c.multiClusterPodController.Get(clusterName, vNamespace, pod.Name)
	if err != nil {
		return fmt.Errorf("could not find pod %s/%s pod in controller cache %v", vNamespace, pod.Name, err)
	}
	var client *clientset.Clientset
	innerCluster := c.multiClusterPodController.GetCluster(clusterName)
	if innerCluster == nil {
		// virtual cluster is gone.
		return nil
	}
	client, err = clientset.NewForConfig(restclient.AddUserAgent(innerCluster.GetClientInfo().Config, "syncer"))
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}

	vPod := vPodObj.(*v1.Pod)
	if vPod.Spec.NodeName != pod.Spec.NodeName {
		n, err := c.client.Nodes().Get(pod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get node %s from super master: %v", pod.Spec.NodeName, err)
		}

		_, err = client.CoreV1().Nodes().Create(node.NewVirtualNode(n))
		if errors.IsAlreadyExists(err) {
			klog.Warningf("virtual node %s already exists", vPod.Spec.NodeName)
		}

		err = client.CoreV1().Pods(vPod.Namespace).Bind(&v1.Binding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vPod.Name,
				Namespace: vPod.Namespace,
			},
			Target: v1.ObjectReference{
				Kind:       "Node",
				Name:       pod.Spec.NodeName,
				APIVersion: "v1",
			},
		})
		if err != nil {
			return fmt.Errorf("failed to bind vPod %s/%s to node %s %v", vPod.Namespace, vPod.Name, pod.Spec.NodeName, err)
		}
		// pod has been updated, return and waiting for next loop.
		return nil
	}
	if !equality.Semantic.DeepEqual(vPod.Status, pod.Status) {
		newPod := vPod.DeepCopy()
		newPod.Status = pod.Status
		if _, err = client.CoreV1().Pods(vPod.Namespace).UpdateStatus(newPod); err != nil {
			return fmt.Errorf("failed to back populate pod %s/%s status update for cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
		}
	}

	return nil
}
