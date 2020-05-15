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
	"k8s.io/apimachinery/pkg/api/equality"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
)

type controller struct {
	// lock to protect nodeNameToCluster
	sync.Mutex
	// phyical node to tenant cluster map. A physical node can be presented as virtual node in multiple tenant clusters.
	nodeNameToCluster map[string]map[string]struct{}
	// super master node client
	nodeClient v1core.NodesGetter
	// super master node lister/synced function
	nodeLister listersv1.NodeLister
	nodeSynced cache.InformerSynced
	// Connect to all tenant master node informers
	multiClusterNodeController *mc.MultiClusterController
	// UWcontroller
	upwardNodeController *uw.UpwardController
}

func Register(
	config *config.SyncerConfiguration,
	client v1core.NodesGetter,
	nodeInformer coreinformers.NodeInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		nodeNameToCluster: make(map[string]map[string]struct{}),
		nodeClient:        client,
	}

	// Create the multi cluster node controller
	options := mc.Options{Reconciler: c, MaxConcurrentReconciles: constants.DwsControllerWorkerLow}
	multiClusterNodeController, err := mc.NewMCController("tenant-masters-node-controller", &v1.Node{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster pod controller %v", err)
		return
	}
	c.multiClusterNodeController = multiClusterNodeController

	c.nodeLister = nodeInformer.Lister()
	c.nodeSynced = nodeInformer.Informer().HasSynced

	uwOptions := &uw.Options{Reconciler: c}
	uwOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerHigh
	upwardNodeController, err := uw.NewUWController("node-upward-controller", &v1.Node{}, *uwOptions)
	if err != nil {
		klog.Errorf("failed to create node upward controller %v", err)
		return
	}
	c.upwardNodeController = upwardNodeController

	nodeInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.enqueueNode,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newNode := newObj.(*v1.Node)
				oldNode := oldObj.(*v1.Node)
				if newNode.ResourceVersion == oldNode.ResourceVersion || equality.Semantic.DeepEqual(newNode.Status.Conditions, oldNode.Status.Conditions) {
					//We only update tenant virtual nodes if there are condition changes, e.g., updating LastHeartBeatTime.
					return
				}

				c.enqueueNode(newObj)
			},
			DeleteFunc: c.enqueueNode,
		},
	)

	controllerManager.AddResourceSyncer(c)
}

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) PatrollerDo() {
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
