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

package endpoints

import (
	v1 "k8s.io/api/core/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

type controller struct {
	// super master endpoints client
	endpointClient v1core.EndpointsGetter
	// super master endpoints informer lister/synced function
	endpointsLister listersv1.EndpointsLister
	endpointsSynced cache.InformerSynced
	// Connect to all tenant master endpoints informers
	multiClusterEndpointsController *mc.MultiClusterController
}

func Register(
	endpointsClient v1core.EndpointsGetter,
	endpointsInformer coreinformers.EndpointsInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		endpointClient: endpointsClient,
	}

	options := mc.Options{Reconciler: c}
	multiClusterEndpointsController, err := mc.NewMCController("tenant-masters-endpoints-controller", &v1.Endpoints{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster endpoints controller %v", err)
		return
	}
	c.multiClusterEndpointsController = multiClusterEndpointsController
	c.endpointsLister = endpointsInformer.Lister()
	c.endpointsSynced = endpointsInformer.Informer().HasSynced

	controllerManager.AddController(c)
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) StartPeriodChecker(stopCh <-chan struct{}) {
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-endpoints-controller watch cluster %s for endpoints resource", cluster.GetClusterName())
	err := c.multiClusterEndpointsController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s endpoints event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-endpoints-controller stop watching cluster %s for endpoints resource", cluster.GetClusterName())
	c.multiClusterEndpointsController.TeardownClusterResource(cluster)
}
