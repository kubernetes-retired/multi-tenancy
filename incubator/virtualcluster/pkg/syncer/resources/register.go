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

package resources

import (
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/configmap"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/endpoints"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/event"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/namespace"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/node"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/persistentvolume"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/persistentvolumeclaim"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/pod"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/secret"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/service"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/serviceaccount"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/storageclass"
)

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []manager.ResourceSyncerNew

func init() {
	AddToManagerFuncs = []manager.ResourceSyncerNew{
		configmap.NewConfigMapController,
		endpoints.NewEndpointsController,
		event.NewEventController,
		namespace.NewNamespaceController,
		node.NewNodeController,
		persistentvolume.NewPVController,
		persistentvolumeclaim.NewPVCController,
		pod.NewPodController,
		secret.NewSecretController,
		service.NewServiceController,
		serviceaccount.NewServiceAccountController,
		storageclass.NewStorageClassController,
	}
}

func Register(config *config.SyncerConfiguration,
	client clientset.Interface,
	informerFactory informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	controllerManager *manager.ControllerManager) error {
	for _, f := range AddToManagerFuncs {
		if c, _, _, err := f(config, client, informerFactory, vcClient, vcInformer, nil); err != nil {
			return err
		} else {
			controllerManager.AddResourceSyncer(c)
		}
	}

	return nil
}
