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
	"sync"
	"time"

	"k8s.io/api/core/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

const DefaultStatusUpdateInterval = 1 * time.Minute

type controller struct {
	sync.Mutex
	clusterToNodeSet           map[string]map[string]*v1.Node
	nodeClient                 v1core.NodesGetter
	multiClusterNodeController *sc.MultiClusterController
}

func Register(
	client v1core.NodesGetter,
	nodeInformer coreinformers.NodeInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		clusterToNodeSet: make(map[string]map[string]*v1.Node),
		nodeClient:       client,
	}

	// Create the multi cluster node controller
	options := sc.Options{Reconciler: c}
	multiClusterNodeController, err := sc.NewController("tenant-masters-node-controller", &v1.Node{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster pod controller %v", err)
		return
	}
	c.multiClusterNodeController = multiClusterNodeController
	controllerManager.AddController(multiClusterNodeController)

	go c.updateNodeStatusLoop()

	// Register the controller as cluster change listener
	listener.AddListener(c)
}

func (c *controller) updateNodeStatusLoop() {
	statusTimer := time.NewTimer(DefaultStatusUpdateInterval)
	defer statusTimer.Stop()

	c.doUpdateNodeStatus()
	for {
		select {
		case <-statusTimer.C:
			c.doUpdateNodeStatus()
			statusTimer.Reset(DefaultStatusUpdateInterval)
		}
	}
}

func (c *controller) doUpdateNodeStatus() {
	c.Lock()
	var wg sync.WaitGroup
	wg.Add(len(c.clusterToNodeSet))
	for clusterName, _ := range c.clusterToNodeSet {
		c.updateClusterNodeStatus(clusterName, c.clusterToNodeSet[clusterName], &wg)
	}
	wg.Wait()
	c.Unlock()
}

func (c *controller) updateClusterNodeStatus(cluster string, nodeSet map[string]*v1.Node, wg *sync.WaitGroup) {
	defer wg.Done()

	innerCluster := c.multiClusterNodeController.GetCluster(cluster)
	client, err := clientset.NewForConfig(restclient.AddUserAgent(innerCluster.GetClientInfo().Config, "syncer"))
	if err != nil {
		klog.Errorf("could not find cluster %s in controller cache %v", cluster, err)
		return
	}

	for nodeName, n := range nodeSet {
		klog.V(4).Infof("updating cluster %s node %s heartbeats", cluster, nodeName)
		updateNodeStatusHeartbeat(n)

		newNode, err := updateNodeStatus(client.CoreV1().Nodes(), n)
		if err != nil {
			klog.Errorf("failed to update cluster %s node %s's heartbeats: %v", cluster, nodeName, err)
			continue
		}

		nodeSet[nodeName] = newNode
	}
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile node %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Node))
		if err != nil {
			klog.Errorf("failed reconcile pod %s/%s in cluster as %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcileUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Node))
		if err != nil {
			return reconciler.Result{}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Node))
		if err != nil {
			return reconciler.Result{}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileCreate(cluster, namespace, name string, node *v1.Node) error {
	c.Lock()
	defer c.Unlock()

	if _, exist := c.clusterToNodeSet[cluster]; !exist {
		c.clusterToNodeSet[cluster] = make(map[string]*v1.Node)
	}
	c.clusterToNodeSet[cluster][name] = node

	return nil
}

func (c *controller) reconcileUpdate(cluster, namespace, name string, node *v1.Node) error {
	c.Lock()
	defer c.Unlock()

	if _, exist := c.clusterToNodeSet[cluster]; !exist {
		c.clusterToNodeSet[cluster] = make(map[string]*v1.Node)
	}
	c.clusterToNodeSet[cluster][name] = node

	return nil
}

func (c *controller) reconcileRemove(cluster, namespace, name string, node *v1.Node) error {
	c.Lock()
	defer c.Unlock()

	if _, exists := c.clusterToNodeSet[cluster]; !exists {
		return nil
	}

	delete(c.clusterToNodeSet[cluster], name)

	if len(c.clusterToNodeSet[cluster]) == 0 {
		delete(c.clusterToNodeSet, cluster)
	}

	return nil
}

func (c *controller) AddCluster(cluster *cluster.Cluster) {
	klog.Infof("tenant-masters-node-controller watch cluster %s for pod resource", cluster.Name)
	err := c.multiClusterNodeController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s node event: %v", cluster.Name, err)
	}
}

func (c *controller) RemoveCluster(cluster *cluster.Cluster) {
	klog.Warningf("not implemented yet")
}
