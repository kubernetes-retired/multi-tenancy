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
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode/native"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
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
	vnodeProvider        vnode.VirtualNodeProvider
}

func NewNodeController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		nodeNameToCluster: make(map[string]map[string]struct{}),
		nodeClient:        client.CoreV1(),
		vnodeProvider:     native.NewNativeVirtualNodeProvider(config.VNAgentPort),
	}

	multiClusterNodeController, err := mc.NewMCController(&v1.Node{}, &v1.NodeList{}, c,
		mc.WithMaxConcurrentReconciles(constants.DwsControllerWorkerLow), mc.WithOptions(options.MCOptions))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create node mc controller: %v", err)
	}
	c.multiClusterNodeController = multiClusterNodeController
	c.nodeLister = informer.Core().V1().Nodes().Lister()
	if options.IsFake {
		c.nodeSynced = func() bool { return true }
	} else {
		c.nodeSynced = informer.Core().V1().Nodes().Informer().HasSynced
	}

	upwardNodeController, err := uw.NewUWController(&v1.Node{}, c,
		uw.WithMaxConcurrentReconciles(constants.UwsControllerWorkerHigh), uw.WithOptions(options.UWOptions))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create node upward controller: %v", err)
	}
	c.upwardNodeController = upwardNodeController

	informer.Core().V1().Nodes().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.enqueueNode,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newNode := newObj.(*v1.Node)
				oldNode := oldObj.(*v1.Node)
				if newNode.ResourceVersion == oldNode.ResourceVersion {
					return
				}

				if equality.Semantic.DeepEqual(newNode.Status.Conditions, oldNode.Status.Conditions) &&
					equality.Semantic.DeepEqual(newNode.Status.Addresses, oldNode.Status.Addresses) {
					// We only update tenant virtual nodes if there are condition or addresses changes, e.g., updating LastHeartBeatTime.
					return
				}

				c.enqueueNode(newObj)
			},
			DeleteFunc: c.enqueueNode,
		},
	)
	return c, multiClusterNodeController, upwardNodeController, nil
}

func (c *controller) SetVNodeProvider(provider vnode.VirtualNodeProvider) {
	c.Lock()
	c.vnodeProvider = provider
	c.Unlock()
}

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) PatrollerDo() {
}

func (c *controller) GetListener() listener.ClusterChangeListener {
	return listener.NewMCControllerListener(c.multiClusterNodeController)
}
