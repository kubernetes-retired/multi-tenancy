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
	"time"

	v1 "k8s.io/api/core/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

type controller struct {
	// super master namespace client
	namespaceClient v1core.NamespacesGetter
	// super master namespace lister
	nsLister listersv1.NamespaceLister
	nsSynced cache.InformerSynced
	// Connect to all tenant master namespace informers
	multiClusterNamespaceController *mc.MultiClusterController
	// Checker timer
	periodCheckerPeriod time.Duration
}

func Register(
	config *config.SyncerConfiguration,
	namespaceClient v1core.CoreV1Interface,
	informer coreinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c, _, err := NewNamespaceController(config, namespaceClient, informer, nil)
	if err != nil {
		klog.Errorf("failed to create multi cluster namespace controller %v", err)
		return
	}

	controllerManager.AddController(c)
}

func NewNamespaceController(config *config.SyncerConfiguration, namespaceClient v1core.CoreV1Interface, informer coreinformers.Interface, options *mc.Options) (manager.Controller, *mc.MultiClusterController, error) {
	c := &controller{
		namespaceClient:     namespaceClient,
		periodCheckerPeriod: 60 * time.Second,
	}
	if options == nil {
		options = &mc.Options{Reconciler: c}
	}
	options.MaxConcurrentReconciles = constants.DwsControllerWorkerLow
	multiClusterNamespaceController, err := mc.NewMCController("tenant-masters-namespace-controller", &v1.Namespace{}, *options)
	if err != nil {
		return nil, nil, err
	}
	c.multiClusterNamespaceController = multiClusterNamespaceController
	c.nsLister = informer.Namespaces().Lister()
	if options.IsFake {
		c.nsSynced = func() bool { return true }
	} else {
		c.nsSynced = informer.Namespaces().Informer().HasSynced
	}

	return c, multiClusterNamespaceController, nil
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
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
