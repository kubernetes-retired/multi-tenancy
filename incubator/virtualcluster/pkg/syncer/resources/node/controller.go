/*
Copyright 2021 The Kubernetes Authors.

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
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode/provider"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/plugin"
)

func init() {
	plugin.SyncerResourceRegister.Register(&plugin.Registration{
		ID: "node",
		InitFn: func(ctx *plugin.InitContext) (interface{}, error) {
			return NewNodeController(ctx.Config.(*config.SyncerConfiguration), ctx.Client, ctx.Informer, ctx.VCClient, ctx.VCInformer, manager.ResourceSyncerOptions{})
		},
	})
}

type controller struct {
	manager.BaseResourceSyncer
	// lock to protect nodeNameToCluster
	sync.Mutex
	// phyical node to tenant cluster map. A physical node can be presented as virtual node in multiple tenant clusters.
	nodeNameToCluster map[string]map[string]struct{}
	// super master node client
	nodeClient v1core.NodesGetter
	// super master node lister/synced function
	nodeLister    listersv1.NodeLister
	nodeSynced    cache.InformerSynced
	vnodeProvider provider.VirtualNodeProvider
}

func NewNodeController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options manager.ResourceSyncerOptions) (manager.ResourceSyncer, error) {
	c := &controller{
		BaseResourceSyncer: manager.BaseResourceSyncer{
			Config: config,
		},
		nodeNameToCluster: make(map[string]map[string]struct{}),
		nodeClient:        client.CoreV1(),
		vnodeProvider:     vnode.GetNodeProvider(config, client),
	}

	var err error
	c.MultiClusterController, err = mc.NewMCController(&v1.Node{}, &v1.NodeList{}, c, mc.WithOptions(options.MCOptions))
	if err != nil {
		return nil, err
	}

	c.nodeLister = informer.Core().V1().Nodes().Lister()
	if options.IsFake {
		c.nodeSynced = func() bool { return true }
	} else {
		c.nodeSynced = informer.Core().V1().Nodes().Informer().HasSynced
	}

	c.UpwardController, err = uw.NewUWController(&v1.Node{}, c,
		uw.WithMaxConcurrentReconciles(constants.UwsControllerWorkerHigh), uw.WithOptions(options.UWOptions))
	if err != nil {
		return nil, err
	}

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
	return c, nil
}

func (c *controller) SetVNodeProvider(provider provider.VirtualNodeProvider) {
	c.Lock()
	c.vnodeProvider = provider
	c.Unlock()
}
