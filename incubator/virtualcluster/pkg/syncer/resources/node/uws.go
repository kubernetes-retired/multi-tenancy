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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.nodeSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.upwardNodeController.Start(stopCh)
}

func (c *controller) enqueueNode(obj interface{}) {
	node := obj.(*v1.Node)
	c.upwardNodeController.AddToQueue(node.Name)
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
	defer metrics.RecordUWSOperationDuration("node", time.Now())
	klog.V(4).Infof("back populate node %s/%s", node.Namespace, node.Name)
	c.Lock()
	clusterList := c.nodeNameToCluster[node.Name]
	c.Unlock()

	if len(clusterList) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	wg.Add(len(clusterList))
	for clusterName, _ := range clusterList {
		go c.updateClusterNodeStatus(clusterName, node, &wg)
	}
	wg.Wait()

	return nil
}

func (c *controller) updateClusterNodeStatus(clusterName string, node *v1.Node, wg *sync.WaitGroup) {
	defer wg.Done()

	tenantClient, err := c.multiClusterNodeController.GetClusterClient(clusterName)
	if err != nil {
		klog.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
		// Cluster is removed. We should remove the entry from nodeNameToCluster map.
		c.Lock()
		delete(c.nodeNameToCluster[node.Name], clusterName)
		c.Unlock()
		return
	}

	vNodeObj, err := c.multiClusterNodeController.Get(clusterName, "", node.Name)
	if err != nil {
		klog.Errorf("could not find node %s/%s: %v", clusterName, node.Name, err)
		return
	}

	vNode := vNodeObj.(*v1.Node)
	newVNode := vNode.DeepCopy()
	newVNode.Status.Conditions = node.Status.Conditions

	_, _, err = patchNodeStatus(tenantClient.CoreV1().Nodes(), types.NodeName(node.Name), vNode, newVNode)
	if err != nil {
		klog.Errorf("failed to update node %s/%s's heartbeats: %v", clusterName, node.Name, err)
		return
	}
}
