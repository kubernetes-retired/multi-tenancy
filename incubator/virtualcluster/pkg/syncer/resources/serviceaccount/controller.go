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

package serviceaccount

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
	// super master sa client
	saClient v1core.CoreV1Interface
	// super master sa lister/synced function
	saLister listersv1.ServiceAccountLister
	saSynced cache.InformerSynced
	// Connect to all tenant master sa informers
	multiClusterServiceAccountController *mc.MultiClusterController
	// Periodic checker
	serviceAccountPatroller *pa.Patroller
}

func Register(
	config *config.SyncerConfiguration,
	client v1core.CoreV1Interface,
	informer coreinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c, _, _, err := NewServiceAccountController(config, client, informer, nil)
	if err != nil {
		klog.Errorf("failed to create multi cluster configmap controller %v", err)
		return
	}

	controllerManager.AddResourceSyncer(c)
}

func NewServiceAccountController(config *config.SyncerConfiguration,
	client v1core.CoreV1Interface,
	informer coreinformers.Interface,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		saClient: client,
	}

	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	mcOptions.MaxConcurrentReconciles = constants.DwsControllerWorkerLow
	multiClusterServiceAccountController, err := mc.NewMCController("tenant-masters-service-account-controller", &v1.ServiceAccount{}, *mcOptions)
	if err != nil {
		return nil, nil, nil, err
	}
	c.multiClusterServiceAccountController = multiClusterServiceAccountController
	c.saLister = informer.ServiceAccounts().Lister()
	if options != nil && options.IsFake {
		c.saSynced = func() bool { return true }
	} else {
		c.saSynced = informer.ServiceAccounts().Informer().HasSynced
	}

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	serviceAccountPatroller, err := pa.NewPatroller("serviceAccount-patroller", *patrolOptions)
	if err != nil {
		klog.Errorf("failed to create serviceAccount patroller %v", err)
		return nil, nil, nil, err
	}
	c.serviceAccountPatroller = serviceAccountPatroller

	return c, multiClusterServiceAccountController, nil, nil
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) BackPopulate(string) error {
	return nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-service-account-controller watch cluster %s for serviceaccount resource", cluster.GetClusterName())
	err := c.multiClusterServiceAccountController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s secret event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-service-account-controller stop watching cluster %s for serviceaccount resource", cluster.GetClusterName())
	c.multiClusterServiceAccountController.TeardownClusterResource(cluster)
}
