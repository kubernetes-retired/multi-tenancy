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
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return c.multiClusterNodeController.Start(stopCh)
}

// The reconcile logic for tenant master node informer, the main purpose is to maintain
// the nodeNameToCluster mapping
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile node %s %s event for cluster %s", request.Name, request.Event, request.Cluster.Name)
	vNode := request.Obj.(*v1.Node)
	if vNode.Labels[constants.LabelVirtualNode] != "true" {
		// We only handle virtual nodes created by syncer
		return reconciler.Result{}, nil
	}

	switch request.Event {
	case reconciler.AddEvent:
		c.reconcileCreate(request.Cluster.Name, request.Namespace, request.Name, vNode)
	case reconciler.UpdateEvent:
		c.reconcileUpdate(request.Cluster.Name, request.Namespace, request.Name, vNode)
	case reconciler.DeleteEvent:
		c.reconcileRemove(request.Cluster.Name, request.Namespace, request.Name, vNode)
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileCreate(cluster, namespace, name string, node *v1.Node) {
	c.Lock()
	defer c.Unlock()

	if _, exist := c.nodeNameToCluster[name]; !exist {
		c.nodeNameToCluster[name] = make(map[string]struct{})
	}
	c.nodeNameToCluster[name][cluster] = struct{}{}
}

func (c *controller) reconcileUpdate(cluster, namespace, name string, node *v1.Node) {
	c.Lock()
	defer c.Unlock()

	if _, exist := c.nodeNameToCluster[name]; !exist {
		c.nodeNameToCluster[name] = make(map[string]struct{})
	}
	c.nodeNameToCluster[name][cluster] = struct{}{}
}

func (c *controller) reconcileRemove(cluster, namespace, name string, node *v1.Node) {
	c.Lock()
	defer c.Unlock()

	if _, exists := c.nodeNameToCluster[name]; exists {
		delete(c.nodeNameToCluster[name], cluster)
	}
}
