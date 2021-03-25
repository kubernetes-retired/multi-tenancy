/*
Copyright 2021 The Kubernetes Authors.

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
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	vclisters "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/featuregate"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/cluster"
	utilconst "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/plugin"
)

var (
	numHealthCluster   uint64
	numUnHealthCluster uint64
)

type Syncer struct {
	config            *config.SyncerConfiguration
	metaClient        clientset.Interface
	superClient       clientset.Interface
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

type virtualclusterGetter struct {
	lister vclisters.VirtualClusterLister
}

var _ mc.Getter = &virtualclusterGetter{}

func (v *virtualclusterGetter) GetObject(namespace, name string) (runtime.Object, error) {
	vc, err := v.lister.VirtualClusters(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	return vc, nil
}

// Bootstrap is a bootstrapping interface for syncer, targets the initialization protocol
type Bootstrap interface {
	ListenAndServe(address, certFile, keyFile string)
	Run(<-chan struct{})
}

func New(
	config *config.SyncerConfiguration,
	virtualClusterClient vcclient.Interface,
	virtualClusterInformer vcinformers.VirtualClusterInformer,
	metaClusterClient clientset.Interface,
	superClusterClient clientset.Interface,
	superClusterInformers informers.SharedInformerFactory,
	recorder record.EventRecorder,
) (*Syncer, error) {
	syncer := &Syncer{
		config:      config,
		metaClient:  metaClusterClient,
		superClient: superClusterClient,
		recorder:    recorder,
		queue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "virtual_cluster"),
		workers:     constants.UwsControllerWorkerLow,
		clusterSet:  make(map[string]mc.ClusterInterface),
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

	plugins := LoadPlugins(config)
	initContext := &plugin.InitContext{
		Context:    context.Background(),
		Config:     config,
		Client:     superClusterClient,
		Informer:   superClusterInformers,
		VCClient:   virtualClusterClient,
		VCInformer: virtualClusterInformer,
	}

	for _, p := range plugins {
		klog.Infof("loading plugin %q...", p.ID)

		result := p.Init(initContext)
		instance, err := result.Instance()
		if err != nil {
			klog.Errorf("failed to load plugin %q", p.ID)
			return nil, err
		}

		s, ok := instance.(manager.ResourceSyncer)
		if ok {
			multiClusterControllerManager.AddResourceSyncer(s)
		} else {
			klog.Warningf("unrecognized plugin %q", p.ID)
		}
	}

	return syncer, nil
}

func LoadPlugins(config *config.SyncerConfiguration) []*plugin.Registration {
	allPlugin := plugin.SyncerResourceRegister.List()
	var enablePlugin []*plugin.Registration
	extraSets := sets.NewString(config.ExtraSyncingResources...)

	for i, r := range allPlugin {
		if !r.Disable || extraSets.Has(r.ID) {
			enablePlugin = append(enablePlugin, allPlugin[i])
		}
	}

	return enablePlugin
}

// enqueue deleted and running object.
func (s *Syncer) enqueueVirtualCluster(obj interface{}) {
	_, ok := obj.(*v1alpha1.VirtualCluster)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		_, ok = tombstone.Obj.(*v1alpha1.VirtualCluster)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a vc %+v", obj))
			return
		}
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
	if featuregate.DefaultFeatureGate.Enabled(featuregate.SuperClusterPooling) {
		klog.Infof("SuperClusterPooling featuregate is enabled!")
		cfg, err := s.superClient.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), utilconst.SuperClusterInfoCfgMap, metav1.GetOptions{})
		if err != nil {
			klog.Infof("Fail to get configmap kube-system/%v from super cluster which is required for SuperClusterPooling feature. Quit!", utilconst.SuperClusterInfoCfgMap)
			os.Exit(1)
		}
		var ok bool
		if utilconst.SuperClusterID, ok = cfg.Data[utilconst.SuperClusterIDKey]; ok == false {
			klog.Infof("Fail to get ID value from configmap kube-system/%v. Quit!", utilconst.SuperClusterInfoCfgMap)
			os.Exit(1)
		}
	}
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

	switch vc.Status.Phase {
	case v1alpha1.ClusterRunning:
		return s.addCluster(key, vc)
	case v1alpha1.ClusterError:
		s.removeCluster(key)
		return nil
	default:
		klog.Infof("Cluster %s/%s not ready to reconcile", vc.Namespace, vc.Name)
		return nil
	}
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

	adminKubeConfigBytes, err := conversion.GetKubeConfigOfVC(s.metaClient.CoreV1(), vc)
	if err != nil {
		return err
	}
	tenantCluster, err := cluster.NewCluster(clusterName, vc.Namespace, vc.Name, string(vc.UID), &virtualclusterGetter{lister: s.lister}, adminKubeConfigBytes, cluster.Options{})
	if err != nil {
		return fmt.Errorf("failed to new tenant cluster %s/%s: %v", vc.Namespace, vc.Name, err)
	}

	// for each resource type of the newly added VirtualCluster, we add the object to informer cache.
	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.AddCluster(tenantCluster)
	}

	s.mu.Lock()
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
	klog.Infof("cluster %s cache sync done", cluster.GetClusterName())

	// start watching cluster resource event after cache sync done.
	for _, clusterChangeListener := range listener.Listeners {
		clusterChangeListener.WatchCluster(cluster)
	}
}

func (s *Syncer) healthPatrol() {
	defer metrics.RecordCheckerScanDuration("TenantMaster", time.Now())
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
