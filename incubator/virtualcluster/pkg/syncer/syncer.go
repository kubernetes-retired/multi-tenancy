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
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	vclisters "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/resources"
)

const (
	KubeconfigAdmin = "admin-kubeconfig"
)

var (
	numHealthCluster   uint64
	numUnHealthCluster uint64
)

type Syncer struct {
	secretClient      v1core.SecretsGetter
	recorder          record.EventRecorder
	controllerManager *manager.ControllerManager
	// lister that can list virtual clusters from a shared cache
	lister vclisters.VirtualClusterLister
	// returns true when the namespace cache is ready
	virtualClusterSynced cache.InformerSynced
	// virtual cluster that have been queued up for processing by workers
	queue   workqueue.RateLimitingInterface
	workers int
	// clusterSet holds the cluster collection in which cluster is running.
	mu         sync.Mutex
	clusterSet map[string]mc.ClusterInterface
}

// Bootstrap is a bootstrapping interface for syncer, targets the initialization protocol
type Bootstrap interface {
	ListenAndServe(address, certFile, keyFile string)
	Run(<-chan struct{})
}

func New(
	config *config.SyncerConfiguration,
	secretClient v1core.SecretsGetter,
	virtualClusterClient vcclient.Interface,
	virtualClusterInformer vcinformers.VirtualClusterInformer,
	superMasterClient clientset.Interface,
	superMasterInformers informers.SharedInformerFactory,
	recorder record.EventRecorder,
) *Syncer {
	syncer := &Syncer{
		secretClient: secretClient,
		recorder:     recorder,
		queue:        workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "virtual_cluster"),
		workers:      constants.UwsControllerWorkerLow,
		clusterSet:   make(map[string]mc.ClusterInterface),
	}

	// Handle VirtualCluster add&delete
	virtualClusterInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: syncer.enqueueVirtualCluster,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newVC := newObj.(*v1alpha1.VirtualCluster)
				oldVC := oldObj.(*v1alpha1.VirtualCluster)
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

	resources.Register(config, superMasterClient, superMasterInformers, virtualClusterClient, virtualClusterInformer, multiClusterControllerManager)

	return syncer
}

// enqueue deleted and running object.
func (s *Syncer) enqueueVirtualCluster(obj interface{}) {
	vc, ok := obj.(*v1alpha1.VirtualCluster)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		vc, ok = tombstone.Obj.(*v1alpha1.VirtualCluster)
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
func (s *Syncer) Run(stopChan <-chan struct{}) {
	go func() {
		if err := s.controllerManager.Start(stopChan); err != nil {
			klog.V(1).Infof("controller manager exit: %v", err)
		}
	}()
	go wait.Until(s.healthPatrol, 1*time.Minute, stopChan)
	go func() {
		defer utilruntime.HandleCrash()
		defer s.queue.ShutDown()

		klog.Infof("starting virtual cluster controller")
		defer klog.Infof("shutting down virtual cluster controller")

		if !cache.WaitForCacheSync(stopChan, s.virtualClusterSynced) {
			return
		}

		klog.V(5).Infof("starting workers")
		for i := 0; i < s.workers; i++ {
			go wait.Until(s.run, 1*time.Second, stopChan)
		}
		<-stopChan
	}()

	return
}

// ListenAndServe initializes a server to respond to HTTP network requests on the syncer.
func (s *Syncer) ListenAndServe(address, certFile, keyFile string) {
	metrics.Register()
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	if certFile != "" && keyFile != "" {
		klog.Fatal(http.ListenAndServeTLS(address, certFile, keyFile, mux))
	} else {
		klog.Fatal(http.ListenAndServe(address, mux))
	}
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

	vc, err := s.lister.VirtualClusters(namespace).Get(name)
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

	vc, exist := s.clusterSet[key]
	if !exist {
		// already deleted
		return
	}

	vc.Stop()

	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.RemoveCluster(vc)
	}

	delete(s.clusterSet, key)
}

// addCluster registers and start an informer cache for the given VirtualCluster
func (s *Syncer) addCluster(key string, vc *v1alpha1.VirtualCluster) error {
	klog.Infof("Add cluster %s", key)

	s.mu.Lock()
	if _, exist := s.clusterSet[key]; exist {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	clusterName := conversion.ToClusterKey(vc)

	var adminKubeConfigBytes []byte
	if adminKubeConfig, exists := vc.GetAnnotations()[constants.LabelAdminKubeConfig]; exists {
		decoded, err := base64.StdEncoding.DecodeString(adminKubeConfig)
		if err != nil {
			return fmt.Errorf("failed to decode kubeconfig from annotations %s: %v", constants.LabelAdminKubeConfig, err)
		}
		adminKubeConfigBytes = decoded
	} else {
		adminKubeConfigSecret, err := s.secretClient.Secrets(clusterName).Get(KubeconfigAdmin, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get secret (%s) for virtual cluster in root namespace %s: %v", KubeconfigAdmin, clusterName, err)
		}
		adminKubeConfigBytes = adminKubeConfigSecret.Data[KubeconfigAdmin]
	}

	tenantCluster, err := cluster.NewTenantCluster(clusterName, vc.Namespace, vc.Name, string(vc.UID), s.lister, adminKubeConfigBytes, cluster.Options{})
	if err != nil {
		return fmt.Errorf("failed to new tenant cluster %s/%s: %v", vc.Namespace, vc.Name, err)
	}

	s.mu.Lock()
	if _, exist := s.clusterSet[key]; exist {
		s.mu.Unlock()
		return nil
	}

	// for each resource type of the newly added VirtualCluster, we add a listener
	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.AddCluster(tenantCluster)
	}

	s.clusterSet[key] = tenantCluster
	s.mu.Unlock()

	go s.runCluster(tenantCluster, vc)

	return nil
}

func (s *Syncer) runCluster(cluster *cluster.Cluster, vc *v1alpha1.VirtualCluster) {
	go func() {
		err := cluster.Start()
		klog.Infof("cluster %s shutdown: %v", cluster.GetClusterName(), err)
	}()

	if !cluster.WaitForCacheSync() {
		s.recorder.Eventf(&v1.ObjectReference{
			Kind:      "VirtualCluster",
			Namespace: vc.Namespace,
			Name:      vc.Name,
			UID:       vc.UID,
		}, v1.EventTypeWarning, "ClusterUnHealth", "VirtualCluster %v unhealth: failed to sync cache", cluster.GetClusterName())

		klog.Warningf("failed to sync cache for cluster %s, retry", cluster.GetClusterName())
		key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(vc)
		s.removeCluster(key)
		s.queue.AddAfter(key, 5*time.Second)
		return
	}
	cluster.SetSynced()
}

func (s *Syncer) healthPatrol() {
	defer metrics.RecordCheckerScanDuration("tenant-master", time.Now())
	var clusters []mc.ClusterInterface
	s.mu.Lock()
	for _, c := range s.clusterSet {
		clusters = append(clusters, c)
	}
	s.mu.Unlock()

	numUnHealthCluster = 0
	numHealthCluster = 0

	if len(clusters) != 0 {
		wg := sync.WaitGroup{}
		for _, c := range clusters {
			wg.Add(1)
			go func(cluster mc.ClusterInterface) {
				defer wg.Done()
				s.checkTenantClusterHealth(cluster)
			}(c)
		}
		wg.Wait()
	}

	metrics.ClusterHealthStats.WithLabelValues("health").Set(float64(numHealthCluster))
	metrics.ClusterHealthStats.WithLabelValues("unhealth").Set(float64(numUnHealthCluster))
}

// checkTenantClusterHealth checks if we can connect to tenant apiserver.
func (s *Syncer) checkTenantClusterHealth(cluster mc.ClusterInterface) {
	cs, err := cluster.GetClientSet()
	if err != nil {
		klog.Warningf("[checkClusterHealth] fails to get cluster %v clientset: %v", cluster.GetClusterName(), err)
		return
	}

	_, discoveryErr := cs.Discovery().ServerVersion()
	if discoveryErr == nil {
		atomic.AddUint64(&numHealthCluster, 1)
		return
	}

	atomic.AddUint64(&numUnHealthCluster, 1)

	ns, name, uid := cluster.GetOwnerInfo()

	s.recorder.Eventf(&v1.ObjectReference{
		Kind:      "VirtualCluster",
		Namespace: ns,
		Name:      name,
		UID:       types.UID(uid),
	}, v1.EventTypeWarning, "ClusterUnHealth", "VirtualCluster %v unhealth: %v", cluster.GetClusterName(), discoveryErr.Error())
}
