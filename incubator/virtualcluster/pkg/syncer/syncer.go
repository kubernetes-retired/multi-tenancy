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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcinformers "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controllers"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
)

const (
	KubeconfigAdmin       = "admin-kubeconfig"
	TLSTimeoutRetryPeriod = 1 * time.Second
)

type Syncer struct {
	secretClient      v1core.SecretsGetter
	controllerManager *manager.ControllerManager
	// if this channel is closed, syncer will stop
	stopChan <-chan struct{}
}

func New(
	secretClient v1core.SecretsGetter,
	virtualClusterInformer vcinformers.VirtualclusterInformer,
	superMasterClient clientset.Interface,
	superMasterInformers informers.SharedInformerFactory,
) *Syncer {
	syncer := &Syncer{
		secretClient: secretClient,
		stopChan:     signals.SetupSignalHandler(),
	}

	// Handle VirtualCluster add&delete
	virtualClusterInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    syncer.onVirtualClusterAdd,
			UpdateFunc: syncer.onVirtualClusterUpdate,
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
		s.controllerManager.Start(s.stopChan)
	}()
}

// registerInformerCache registers and start an informer cache for the
// given Virtualcluster
func (s *Syncer) registerInformerCache(vc *v1alpha1.Virtualcluster) (err error) {
	// if a Virtualcluster is starting to run, build a cluster admin client
	// based on the admin-kubeconfig secret of the Virtualcluster.
	klog.Infof("Virtualcluster(%s) is running", vc.Name)
	adminKubeconfigSecret, err :=
		s.secretClient.Secrets(vc.Namespace).Get(KubeconfigAdmin, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("failed to get secret (%s) for virtual cluster %s: %v",
			KubeconfigAdmin, vc.Name, err)
		return
	}
	clusterRestConfig, err :=
		clientcmd.RESTConfigFromKubeConfig(adminKubeconfigSecret.Data[KubeconfigAdmin])
	if err != nil {
		klog.Errorf("failed to build rest config for virtual cluster %s: %v", vc.Name, err)
		return
	}
	innerCluster := &cluster.Cluster{
		Name:   vc.Name,
		Config: clusterRestConfig,
	}

	// for each resource type of the newly added Virtualcluster, we add a listener
	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.AddCluster(innerCluster)
	}

	// start the informer cach of the newly added Virtualcluster
	klog.Errorf("starting informer cache for Virtualcluster(%s)", innerCluster.Name)
	err = innerCluster.Start(s.stopChan)
	if err != nil {
		klog.Errorf("fail to start Virtualcluster(%s) cache", innerCluster.Name)
	}
	return
}

// onVirtualClusterAdd sets up informer for existing Virtualclusters
func (s *Syncer) onVirtualClusterAdd(obj interface{}) {
	vc, ok := obj.(*v1alpha1.Virtualcluster)
	if !ok {
		klog.Errorf("cannot convert to *v1alpha1.VirtualCluster: %v", obj)
		return
	}
	// only register informer cache for Virtualcluster that is running
	if vc.Status.Phase == v1alpha1.ClusterRunning {
		if err := s.registerInformerCache(vc); err != nil {
			klog.Errorf("fail to register informer cache for Virtualcluster(%s): %s", vc.Name, err)
			return
		}
	}
}

// onVirtualClusterUpdate checks if a Virtualcluster is starting to run. For a
// newly started Virtualcluster, we will register it with all resources
// controllers (e.g. pod, node, etc.).
func (s *Syncer) onVirtualClusterUpdate(old, new interface{}) {
	oldVc, ok := old.(*v1alpha1.Virtualcluster)
	if !ok {
		klog.Errorf("cannot convert to *v1alpha1.VirtualCluster: %v", old)
		return
	}
	vc, ok := new.(*v1alpha1.Virtualcluster)
	if !ok {
		klog.Errorf("cannot convert to *v1alpha1.VirtualCluster: %v", new)
		return
	}

	switch {
	case vc.Status.Phase == v1alpha1.ClusterPending:
		klog.Infof("Virtualcluster(%s) is pending", vc.Name)
	case vc.Status.Phase == v1alpha1.ClusterRunning &&
		oldVc.Status.Phase != v1alpha1.ClusterRunning:
		if err := s.registerInformerCache(vc); err != nil {
			klog.Errorf("fail to register informer cache for Virtualcluster(%s): %s", vc.Name, err)
			return
		}
	default:
		klog.Errorf("unknown Virtualcluster(%s) phase: %s",
			vc.Name, vc.Status.Phase)
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
