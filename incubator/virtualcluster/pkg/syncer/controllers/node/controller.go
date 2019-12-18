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

	v1 "k8s.io/api/core/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type controller struct {
	sync.Mutex
	// map node name in super master to tenant cluster name it belongs to.
	nodeNameToCluster          map[string]map[string]struct{}
	nodeClient                 v1core.NodesGetter
	multiClusterNodeController *mc.MultiClusterController

	workers    int
	nodeLister listersv1.NodeLister
	queue      workqueue.RateLimitingInterface
	nodeSynced cache.InformerSynced
}

func Register(
	client v1core.NodesGetter,
	nodeInformer coreinformers.NodeInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		nodeNameToCluster: make(map[string]map[string]struct{}),
		nodeClient:        client,
		queue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "super_master_node"),
		workers:           constants.DefaultControllerWorkers,
	}

	// Create the multi cluster node controller
	options := mc.Options{Reconciler: c}
	multiClusterNodeController, err := mc.NewMCController("tenant-masters-node-controller", &v1.Node{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster pod controller %v", err)
		return
	}
	c.multiClusterNodeController = multiClusterNodeController

	c.nodeLister = nodeInformer.Lister()
	c.nodeSynced = nodeInformer.Informer().HasSynced
	nodeInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.enqueueNode,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newNode := newObj.(*v1.Node)
				oldNode := oldObj.(*v1.Node)
				if newNode.ResourceVersion == oldNode.ResourceVersion {
					return
				}

				c.enqueueNode(newObj)
			},
			DeleteFunc: c.enqueueNode,
		},
	)

	controllerManager.AddController(c)
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return c.multiClusterNodeController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile node %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Node))
		if err != nil {
			klog.Errorf("failed reconcile node %s/%s in cluster %s as %v", request.Namespace, request.Name, request.Cluster.Name, err)
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

	if _, exist := c.nodeNameToCluster[name]; !exist {
		c.nodeNameToCluster[name] = make(map[string]struct{})
	}
	c.nodeNameToCluster[name][cluster] = struct{}{}

	return nil
}

func (c *controller) reconcileUpdate(cluster, namespace, name string, node *v1.Node) error {
	c.Lock()
	defer c.Unlock()

	if _, exist := c.nodeNameToCluster[name]; !exist {
		c.nodeNameToCluster[name] = make(map[string]struct{})
	}
	c.nodeNameToCluster[name][cluster] = struct{}{}

	return nil
}

func (c *controller) reconcileRemove(cluster, namespace, name string, node *v1.Node) error {
	c.Lock()
	defer c.Unlock()

	if _, exists := c.nodeNameToCluster[name]; !exists {
		return nil
	}

	delete(c.nodeNameToCluster[name], cluster)

	return nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-node-controller watch cluster %s for node resource", cluster.GetClusterName())
	err := c.multiClusterNodeController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s node event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-node-controller stop watching cluster %s for node resource", cluster.GetClusterName())
	c.multiClusterNodeController.TeardownClusterResource(cluster)
}
