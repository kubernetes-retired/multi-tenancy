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
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	vcclient "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	vclisters "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	pa "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
)

type controller struct {
	// super master namespace client
	namespaceClient v1core.NamespacesGetter
	// super master namespace lister
	nsLister listersv1.NamespaceLister
	nsSynced cache.InformerSynced
	// super master virtual cluster lister
	vcClient vcclient.Interface
	vcLister vclisters.VirtualclusterLister
	vcSynced cache.InformerSynced
	// Connect to all tenant master namespace informers
	multiClusterNamespaceController *mc.MultiClusterController
	// Periodic checker
	namespacePatroller *pa.Patroller
}

func Register(
	config *config.SyncerConfiguration,
	namespaceClient v1core.CoreV1Interface,
	informer coreinformers.Interface,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualclusterInformer,
	controllerManager *manager.ControllerManager,
) {
	c, _, _, err := NewNamespaceController(config, namespaceClient, informer, vcClient, vcInformer, nil)
	if err != nil {
		klog.Errorf("failed to create multi cluster namespace controller %v", err)
		return
	}
	controllerManager.AddResourceSyncer(c)
}

func NewNamespaceController(config *config.SyncerConfiguration,
	namespaceClient v1core.CoreV1Interface,
	informer coreinformers.Interface,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualclusterInformer,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		namespaceClient: namespaceClient,
		vcClient:        vcClient,
	}
	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	mcOptions.MaxConcurrentReconciles = constants.DwsControllerWorkerLow
	multiClusterNamespaceController, err := mc.NewMCController("tenant-masters-namespace-controller", &v1.Namespace{}, *mcOptions)
	if err != nil {
		return nil, nil, nil, err
	}
	c.multiClusterNamespaceController = multiClusterNamespaceController
	c.nsLister = informer.Namespaces().Lister()
	c.vcLister = vcInformer.Lister()
	if options != nil && options.IsFake {
		c.nsSynced = func() bool { return true }
		c.vcSynced = func() bool { return true }
	} else {
		c.nsSynced = informer.Namespaces().Informer().HasSynced
		c.vcSynced = vcInformer.Informer().HasSynced
	}

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	namespacePatroller, err := pa.NewPatroller("namespace-patroller", *patrolOptions)
	if err != nil {
		klog.Errorf("failed to create namespace patroller %v", err)
		return nil, nil, nil, err
	}
	c.namespacePatroller = namespacePatroller

	return c, multiClusterNamespaceController, nil, nil
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) BackPopulate(string) error {
	return nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-namespace-controller watch cluster %s for namespace resource", cluster.GetClusterName())
	err := c.multiClusterNamespaceController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s namespace event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-namespace-controller stop watching cluster %s for namespace resource", cluster.GetClusterName())
	c.multiClusterNamespaceController.TeardownClusterResource(cluster)
}
