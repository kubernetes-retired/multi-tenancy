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

package configmap

import (
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
	pa "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
)

type controller struct {
	config *config.SyncerConfiguration
	// super master configMap client
	configMapClient v1core.ConfigMapsGetter
	// super master configMap informer lister/synced function
	configMapLister listersv1.ConfigMapLister
	configMapSynced cache.InformerSynced
	// Connect to all tenant master configMap informers
	multiClusterConfigMapController *mc.MultiClusterController
	// Periodic checker
	configMapPatroller *pa.Patroller
}

func Register(
	config *config.SyncerConfiguration,
	configMapClient v1core.ConfigMapsGetter,
	configMapInformer coreinformers.ConfigMapInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		config:          config,
		configMapClient: configMapClient,
	}

	// Create the multi cluster configmap controller
	options := mc.Options{Reconciler: c, MaxConcurrentReconciles: constants.DwsControllerWorkerLow}
	multiClusterConfigMapController, err := mc.NewMCController("tenant-masters-configmap-controller", &v1.ConfigMap{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster configmap controller %v", err)
		return
	}
	c.multiClusterConfigMapController = multiClusterConfigMapController
	c.configMapLister = configMapInformer.Lister()
	c.configMapSynced = configMapInformer.Informer().HasSynced

	patrolOptions := &pa.Options{Reconciler: c}
	configMapPatroller, err := pa.NewPatroller("configMap-patroller", *patrolOptions)
	if err != nil {
		klog.Errorf("failed to create configMap patroller %v", err)
		return
	}
	c.configMapPatroller = configMapPatroller

	controllerManager.AddResourceSyncer(c)
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) BackPopulate(string) error {
	return nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-configmap-controller watch cluster %s for configmap resource", cluster.GetClusterName())
	err := c.multiClusterConfigMapController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s configmap event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-configmap-controller stop watching cluster %s for configmap resource", cluster.GetClusterName())
	c.multiClusterConfigMapController.TeardownClusterResource(cluster)
}
