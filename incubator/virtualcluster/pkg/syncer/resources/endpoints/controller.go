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

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
)

type controller struct {
	config *config.SyncerConfiguration
	// super master endpoints client
	endpointClient v1core.EndpointsGetter
	// super master endpoints informer lister/synced function
	endpointsLister listersv1.EndpointsLister
	endpointsSynced cache.InformerSynced
	// Connect to all tenant master endpoints informers
	multiClusterEndpointsController *mc.MultiClusterController
	// Periodic checker
	endPointsPatroller *pa.Patroller
}

func Register(
	config *config.SyncerConfiguration,
	client v1core.CoreV1Interface,
	informer coreinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c, _, _, err := NewEndpointsController(config, client, informer, nil)
	if err != nil {
		klog.Errorf("failed to create multi cluster endpoints controller %v", err)
		return
	}

	controllerManager.AddResourceSyncer(c)
}

func NewEndpointsController(config *config.SyncerConfiguration,
	client v1core.CoreV1Interface,
	informer coreinformers.Interface,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		config:         config,
		endpointClient: client,
	}

	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	mcOptions.MaxConcurrentReconciles = constants.DwsControllerWorkerLow
	multiClusterEndpointsController, err := mc.NewMCController("tenant-masters-endpoints-controller", &v1.Endpoints{}, *mcOptions)
	if err != nil {
		return nil, nil, nil, err
	}
	c.multiClusterEndpointsController = multiClusterEndpointsController
	c.endpointsLister = informer.Endpoints().Lister()
	if options != nil && options.IsFake {
		c.endpointsSynced = func() bool { return true }
	} else {
		c.endpointsSynced = informer.Endpoints().Informer().HasSynced
	}

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	endPointsPatroller, err := pa.NewPatroller("endPoints-patroller", *patrolOptions)
	if err != nil {
		klog.Errorf("failed to create endpoints patroller %v", err)
		return nil, nil, nil, err
	}
	c.endPointsPatroller = endPointsPatroller

	return c, multiClusterEndpointsController, nil, nil
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) BackPopulate(string) error {
	return nil
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
