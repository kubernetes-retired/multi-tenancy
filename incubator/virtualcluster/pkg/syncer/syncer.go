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

package syncer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/klog"

	"github.com/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcinformers "github.com/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"github.com/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	"github.com/multi-tenancy/incubator/virtualcluster/pkg/syncer/controllers"
	"github.com/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

const (
	KubeconfigAdmin = "admin.kubeconfig"
)

type Syncer struct {
	secretClient      v1core.SecretsGetter
	controllerManager *manager.ControllerManager
}

func New(
	secretClient v1core.SecretsGetter,
	virtualClusterInformer vcinformers.VirtualclusterInformer,
	superMasterClient clientset.Interface,
	superMasterInformers informers.SharedInformerFactory,
) *Syncer {
	syncer := &Syncer{
		secretClient: secretClient,
	}

	// Handle VirtualCluster add&delete
	virtualClusterInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    syncer.onVirtualClusterAdd,
			DeleteFunc: syncer.onVirtualClusterDelete,
		},
	)

	// Create the multi cluster controller manager
	multiClusterControllerManager := manager.New()
	syncer.controllerManager = multiClusterControllerManager

	controllers.Register(superMasterClient, superMasterInformers, multiClusterControllerManager)

	return syncer
}

// Run begins watching and downward&upward syncing.
func (s *Syncer) Run() {
	go func() {
		s.controllerManager.Start(signals.SetupSignalHandler())
	}()
}

func (s *Syncer) onVirtualClusterAdd(obj interface{}) {
	vc, ok := obj.(*v1alpha1.Virtualcluster)
	if !ok {
		klog.Errorf("cannot convert to *v1alpha1.VirtualCluster: %v", obj)
		return
	}

	klog.Infof("handle virtual cluster %s add event", vc.Name)

	// Build cluster admin client based on admin.kubeconfig secret
	adminKubeconfigSecret, err := s.secretClient.Secrets(vc.Name).Get(KubeconfigAdmin, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("failed to get admin.kubeconfig secret for virtual cluster %s", vc.Name)
		return
	}
	clusterRestConfig, err := clientcmd.RESTConfigFromKubeConfig(adminKubeconfigSecret.Data[KubeconfigAdmin])
	if err != nil {
		klog.Errorf("failed to build rest config for virtual cluster %s", vc.Name)
		return
	}

	innerCluster := &cluster.Cluster{
		Name:   vc.Name,
		Config: clusterRestConfig,
	}
	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.AddCluster(innerCluster)
	}
}

func (s *Syncer) onVirtualClusterDelete(obj interface{}) {
	var vc *v1alpha1.Virtualcluster
	switch t := obj.(type) {
	case *v1alpha1.Virtualcluster:
		vc = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		vc, ok = t.Obj.(*v1alpha1.Virtualcluster)
		if !ok {
			klog.Errorf("cannot convert to *v1.Node: %v", t.Obj)
			return
		}
	default:
		klog.Errorf("cannot convert to *v1.Node: %v", t)
		return
	}

	klog.Infof("handle virtual cluster %s delete event", vc.Name)

	innerCluster := &cluster.Cluster{
		Name: vc.Name,
	}
	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.RemoveCluster(innerCluster)
	}
}
