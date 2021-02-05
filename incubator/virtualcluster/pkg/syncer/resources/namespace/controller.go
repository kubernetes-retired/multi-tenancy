/*
Copyright 2020 The Kubernetes Authors.

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

package namespace

import (
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	vclisters "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/plugin"
)

func init() {
	plugin.Register(&plugin.Registration{
		ID: "namespace",
		InitFn: func(ctx *plugin.InitContext) (interface{}, error) {
			return NewNamespaceController(ctx.Config.(*config.SyncerConfiguration), ctx.Client, ctx.Informer, ctx.VCClient, ctx.VCInformer, manager.ResourceSyncerOptions{})
		},
	})
}

type controller struct {
	manager.BaseResourceSyncer
	// super master namespace client
	namespaceClient v1core.NamespacesGetter
	// super master namespace lister
	nsLister listersv1.NamespaceLister
	nsSynced cache.InformerSynced
	// super master virtual cluster lister
	vcClient vcclient.Interface
	vcLister vclisters.VirtualClusterLister
	vcSynced cache.InformerSynced
}

func NewNamespaceController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options manager.ResourceSyncerOptions) (manager.ResourceSyncer, error) {
	c := &controller{
		BaseResourceSyncer: manager.BaseResourceSyncer{
			Config: config,
		},
		namespaceClient: client.CoreV1(),
		vcClient:        vcClient,
	}

	var err error
	c.MultiClusterController, err = mc.NewMCController(&v1.Namespace{}, &v1.NamespaceList{}, c, mc.WithOptions(options.MCOptions))
	if err != nil {
		return nil, err
	}

	c.nsLister = informer.Core().V1().Namespaces().Lister()
	c.vcLister = vcInformer.Lister()
	if options.IsFake {
		c.nsSynced = func() bool { return true }
		c.vcSynced = func() bool { return true }
	} else {
		c.nsSynced = informer.Core().V1().Namespaces().Informer().HasSynced
		c.vcSynced = vcInformer.Informer().HasSynced
	}

	c.Patroller, err = pa.NewPatroller(&v1.Namespace{}, c, pa.WithOptions(options.PatrolOptions))
	if err != nil {
		return nil, err
	}

	return c, nil
}
