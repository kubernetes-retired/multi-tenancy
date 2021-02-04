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

package node

import (
	"fmt"
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.nodeSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.UpwardController.Start(stopCh)
}

func (c *controller) enqueueNode(obj interface{}) {
	node := obj.(*v1.Node)
	c.UpwardController.AddToQueue(node.Name)
}

func (c *controller) BackPopulate(nodeName string) error {
	node, err := c.nodeLister.Get(nodeName)
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO: notify every tenant.
			return nil
		}
		return err
	}
	klog.V(4).Infof("back populate node %s/%s", node.Namespace, node.Name)
	c.Lock()
	var clusterList []string
	for clusterName := range c.nodeNameToCluster[node.Name] {
		clusterList = append(clusterList, clusterName)
	}
	c.Unlock()

	if len(clusterList) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(len(clusterList))
	for _, clusterName := range clusterList {
		go c.updateClusterNodeStatus(clusterName, node, &wg)
	}
	wg.Wait()

	return nil
}

func (c *controller) updateClusterNodeStatus(clusterName string, node *v1.Node, wg *sync.WaitGroup) {
	defer wg.Done()

	tenantClient, err := c.MultiClusterController.GetClusterClient(clusterName)
	if err != nil {
		klog.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
		// Cluster is removed. We should remove the entry from nodeNameToCluster map.
		c.Lock()
		delete(c.nodeNameToCluster[node.Name], clusterName)
		c.Unlock()
		return
	}

	vNodeObj, err := c.MultiClusterController.Get(clusterName, "", node.Name)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Errorf("could not find node %s/%s: %v", clusterName, node.Name, err)
			c.Lock()
			if _, exists := c.nodeNameToCluster[node.Name]; exists {
				delete(c.nodeNameToCluster[node.Name], clusterName)
			}
			c.Unlock()
		}
		return
	}

	vNode := vNodeObj.(*v1.Node)
	newVNode := vNode.DeepCopy()
	newVNode.Status.Conditions = node.Status.Conditions
	vNodeAddress, err := c.vnodeProvider.GetNodeAddress(node)
	if err != nil {
		klog.Errorf("unable get node address from provider: %v", err)
		return
	}
	newVNode.Status.Addresses = vNodeAddress

	if err := vnode.UpdateNodeStatus(tenantClient.CoreV1().Nodes(), vNode, newVNode); err != nil {
		klog.Errorf("failed to update node %s/%s's heartbeats: %v", clusterName, node.Name, err)
	}
	return
}
