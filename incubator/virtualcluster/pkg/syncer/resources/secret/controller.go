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

package secret

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
	// super master secret client
	secretClient v1core.CoreV1Interface
	// super master secret informer/lister/synced function
	secretInformer coreinformers.SecretInformer
	secretLister   listersv1.SecretLister
	secretSynced   cache.InformerSynced
	// Connect to all tenant master secret informers
	multiClusterSecretController *mc.MultiClusterController
	// Periodic checker
	secretPatroller *pa.Patroller
}

func Register(
	config *config.SyncerConfiguration,
	secretClient v1core.CoreV1Interface,
	secretInformer coreinformers.SecretInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		secretClient:   secretClient,
		secretInformer: secretInformer,
	}

	// Create the multi cluster secret controller
	options := mc.Options{Reconciler: c, MaxConcurrentReconciles: constants.DwsControllerWorkerLow}
	multiClusterSecretController, err := mc.NewMCController("tenant-masters-secret-controller", &v1.Secret{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster secret controller %v", err)
		return
	}
	c.multiClusterSecretController = multiClusterSecretController

	c.secretLister = secretInformer.Lister()
	c.secretSynced = secretInformer.Informer().HasSynced

	patrolOptions := &pa.Options{Reconciler: c}
	secretPatroller, err := pa.NewPatroller("secret-patroller", *patrolOptions)
	if err != nil {
		klog.Errorf("failed to create secret patroller %v", err)
		return
	}
	c.secretPatroller = secretPatroller

	controllerManager.AddResourceSyncer(c)
}

func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	return nil
}

func (c *controller) BackPopulate(string) error {
	return nil
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-secret-controller watch cluster %s for secret resource", cluster.GetClusterName())
	err := c.multiClusterSecretController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s secret event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-secret-controller stop watching cluster %s for secret resource", cluster.GetClusterName())
	c.multiClusterSecretController.TeardownClusterResource(cluster)
}
