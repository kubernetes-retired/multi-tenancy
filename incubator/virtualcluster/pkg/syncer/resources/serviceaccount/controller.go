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
	"k8s.io/api/core/v1"
	coreinformers "k8s.io/client-go/informers/core/v1"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

type controller struct {
	// super master sa client
	saClient v1core.CoreV1Interface
	// super master sa lister/synced function
	saLister listersv1.ServiceAccountLister
	saSynced cache.InformerSynced
	// Connect to all tenant master sa informers
	multiClusterServiceAccountController *mc.MultiClusterController
}

func Register(
	config *config.SyncerConfiguration,
	client v1core.CoreV1Interface,
	saInformer coreinformers.ServiceAccountInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		saClient: client,
	}

	// Create the multi cluster secret controller
	options := mc.Options{Reconciler: c}
	multiClusterSecretController, err := mc.NewMCController("tenant-masters-service-account-controller", &v1.ServiceAccount{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster secret controller %v", err)
		return
	}
	c.multiClusterServiceAccountController = multiClusterSecretController
	c.saLister = saInformer.Lister()
	c.saSynced = saInformer.Informer().HasSynced

	controllerManager.AddController(c)
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) StartPeriodChecker(stopCh <-chan struct{}) error {
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
