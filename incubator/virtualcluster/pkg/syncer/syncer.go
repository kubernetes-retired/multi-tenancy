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
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcinformers "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	vclisters "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controllers"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
)

const (
	KubeconfigAdmin = "admin-kubeconfig"
)

type Syncer struct {
	secretClient      v1core.SecretsGetter
	controllerManager *manager.ControllerManager
	// lister that can list virtual clusters from a shared cache
	lister vclisters.VirtualclusterLister
	// returns true when the namespace cache is ready
	virtualClusterSynced cache.InformerSynced
	// virtual cluster that have been queued up for processing by workers
	queue   workqueue.RateLimitingInterface
	workers int
	// clusterSet holds the cluster name collection in which cluster is running.
	mu         sync.Mutex
	clusterSet sets.String
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
		queue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "virtual_cluster"),
		workers:      constants.DefaultControllerWorkers,
		clusterSet:   sets.NewString(),
		stopChan:     signals.SetupSignalHandler(),
	}

	// Handle VirtualCluster add&delete
	virtualClusterInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: syncer.enqueueVirtualCluster,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newVC := newObj.(*v1alpha1.Virtualcluster)
				oldVC := oldObj.(*v1alpha1.Virtualcluster)
				if newVC.ResourceVersion == oldVC.ResourceVersion {
					return
				}
				syncer.enqueueVirtualCluster(newObj)
			},
			DeleteFunc: syncer.enqueueVirtualCluster,
		},
	)
	syncer.lister = virtualClusterInformer.Lister()
	syncer.virtualClusterSynced = virtualClusterInformer.Informer().HasSynced

	// Create the multi cluster controller manager
	multiClusterControllerManager := manager.New()
	syncer.controllerManager = multiClusterControllerManager

	controllers.Register(superMasterClient, superMasterInformers, multiClusterControllerManager)

	return syncer
}

// enqueue deleted and running object.
func (s *Syncer) enqueueVirtualCluster(obj interface{}) {
	vc, ok := obj.(*v1alpha1.Virtualcluster)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		vc, ok = tombstone.Obj.(*v1alpha1.Virtualcluster)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a vc %+v", obj))
			return
		}
	}

	if vc.Status.Phase != v1alpha1.ClusterRunning {
		return
	}

	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	s.queue.Add(key)
}

// Run begins watching and downward&upward syncing.
func (s *Syncer) Run() {
	go func() {
		if err := s.controllerManager.Start(s.stopChan); err != nil {
			klog.V(1).Infof("controller manager exit: %v", err)
		}
	}()
	go func() {
		defer utilruntime.HandleCrash()
		defer s.queue.ShutDown()

		klog.Infof("starting virtual cluster controller")
		defer klog.Infof("shutting down virtual cluster controller")

		if !cache.WaitForCacheSync(s.stopChan, s.virtualClusterSynced) {
			return
		}

		klog.V(5).Infof("starting workers")
		for i := 0; i < s.workers; i++ {
			go wait.Until(s.run, 1*time.Second, s.stopChan)
		}
		<-s.stopChan
	}()

	return
}

// run runs a run thread that just dequeues items, processes them, and marks them done.
// It enforces that the syncHandler is never invoked concurrently with the same key.
func (s *Syncer) run() {
	for s.processNextWorkItem() {
	}
}

func (s *Syncer) processNextWorkItem() bool {
	key, quit := s.queue.Get()
	if quit {
		return false
	}
	defer s.queue.Done(key)

	err := s.syncVirtualCluster(key.(string))
	if err == nil {
		s.queue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing virtual cluster %v (will retry): %v", key, err))
	s.queue.AddRateLimited(key)
	return true
}

func (s *Syncer) syncVirtualCluster(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	vc, err := s.lister.Virtualclusters(namespace).Get(name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		s.removeCluster(key)
		return nil
	}

	return s.addCluster(key, vc)
}

func (s *Syncer) removeCluster(key string) {
	klog.Infof("Remove cluster %s", key)

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.clusterSet.Has(key) {
		// already deleted
		return
	}

	innerCluster := &cluster.Cluster{
		Name: key,
	}
	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.RemoveCluster(innerCluster)
	}

	s.clusterSet.Delete(key)
}

// addCluster registers and start an informer cache for the given VirtualCluster
func (s *Syncer) addCluster(key string, vc *v1alpha1.Virtualcluster) error {
	klog.Infof("Add cluster %s", key)

	s.mu.Lock()
	if s.clusterSet.Has(key) {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	adminKubeConfigSecret, err := s.secretClient.Secrets(vc.Namespace).Get(KubeconfigAdmin, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret (%s) for virtual cluster %s/%s: %v", KubeconfigAdmin, vc.Namespace, vc.Name, err)
	}
	clusterRestConfig, err := clientcmd.RESTConfigFromKubeConfig(adminKubeConfigSecret.Data[KubeconfigAdmin])
	if err != nil {
		return fmt.Errorf("failed to build rest config for virtual cluster %s/%s: %v", vc.Namespace, vc.Name, err)
	}
	innerCluster := &cluster.Cluster{
		Name:   conversion.ToClusterKey(vc),
		Config: clusterRestConfig,
	}

	s.mu.Lock()
	if s.clusterSet.Has(key) {
		s.mu.Unlock()
		return nil
	}

	// for each resource type of the newly added VirtualCluster, we add a listener
	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.AddCluster(innerCluster)
	}
	s.clusterSet.Insert(key)
	s.mu.Unlock()

	go func() {
		if err = innerCluster.Start(s.stopChan); err != nil {
			s.removeCluster(key)
			// retry if start cluster fails.
			s.queue.AddAfter(key, 5*time.Second)
		}
	}()

	return nil
}
