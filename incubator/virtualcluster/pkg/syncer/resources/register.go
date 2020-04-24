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
	vcclient "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/configmap"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/endpoints"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/event"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/namespace"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/node"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/persistentvolume"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/persistentvolumeclaim"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/pod"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/secret"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/service"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/serviceaccount"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources/storageclass"
)

func Register(config *config.SyncerConfiguration,
	client clientset.Interface,
	informerFactory informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualclusterInformer,
	controllerManager *manager.ControllerManager) {
	namespace.Register(config, client.CoreV1(), informerFactory.Core().V1(), vcClient, vcInformer, controllerManager)
	pod.Register(config, client.CoreV1(), informerFactory.Core().V1(), controllerManager)
	configmap.Register(config, client.CoreV1(), informerFactory.Core().V1(), controllerManager)
	secret.Register(config, client.CoreV1(), informerFactory.Core().V1(), controllerManager)
	serviceaccount.Register(config, client.CoreV1(), informerFactory.Core().V1().ServiceAccounts(), controllerManager)
	node.Register(config, client.CoreV1(), informerFactory.Core().V1().Nodes(), controllerManager)
	service.Register(config, client.CoreV1(), informerFactory.Core().V1(), controllerManager)
	endpoints.Register(config, client.CoreV1(), informerFactory.Core().V1().Endpoints(), controllerManager)
	event.Register(config, client.CoreV1(), informerFactory.Core().V1(), controllerManager)
	storageclass.Register(config, client.StorageV1(), informerFactory.Storage().V1(), controllerManager)
	persistentvolumeclaim.Register(config, client.CoreV1(), informerFactory.Core().V1(), controllerManager)
	persistentvolume.Register(config, client.CoreV1(), informerFactory.Core().V1(), controllerManager)
}
