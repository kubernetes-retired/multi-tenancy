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

package scheduler

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/apis/cluster/v1alpha4"
	superclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/clientset/versioned"
	superinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/informers/externalversions/cluster/v1alpha4"
	superLister "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/listers/cluster/v1alpha4"
	schedulerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/apis/config"
	internalcache "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/cache"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/manager"
	virtualClusterWatchers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/resource/virtualcluster"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/util"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	virtualClusterLister "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/plugin"
)

var SuperClusterResourceRegister plugin.ResourceRegister

type Scheduler struct {
	config            *schedulerconfig.SchedulerConfiguration
	metaClusterClient clientset.Interface
	recorder          record.EventRecorder

	superClusterWatcher *manager.WatchManager
	superClusterLister  superLister.ClusterLister
	superClusterSynced  cache.InformerSynced
	superClusterQueue   workqueue.RateLimitingInterface
	superClusterWorkers int
	superClusterLock    sync.Mutex
	superClusterSet     map[string]mc.ClusterInterface

	virtualClusterWatcher *manager.WatchManager
	virtualClusterLister  virtualClusterLister.VirtualClusterLister
	virtualClusterSynced  cache.InformerSynced
	virtualClusterQueue   workqueue.RateLimitingInterface
	virtualClusterWorkers int
	virtualClusterLock    sync.Mutex
	virtualClusterSet     map[string]mc.ClusterInterface

	schedulerCache internalcache.Cache
}

func New(
	config *schedulerconfig.SchedulerConfiguration,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	superClient superclient.Interface,
	superInformer superinformers.ClusterInformer,
	metaClusterClient clientset.Interface,
	metaInformers informers.SharedInformerFactory,
	stopCh <-chan struct{},
	recorder record.EventRecorder,
) (*Scheduler, error) {
	scheduler := &Scheduler{
		config:                config,
		metaClusterClient:     metaClusterClient,
		recorder:              recorder,
		virtualClusterQueue:   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "virtualcluster"),
		virtualClusterWorkers: constants.VirtualClusterWorker,
		virtualClusterSet:     make(map[string]mc.ClusterInterface),
		superClusterQueue:     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "supercluster"),
		superClusterWorkers:   constants.SuperClusterWorker,
		superClusterSet:       make(map[string]mc.ClusterInterface),
	}

	// Handle VirtualCluster add&delete
	vcInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: scheduler.enqueueVirtualCluster,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newVC := newObj.(*v1alpha1.VirtualCluster)
				oldVC := oldObj.(*v1alpha1.VirtualCluster)
				if newVC.ResourceVersion == oldVC.ResourceVersion {
					return
				}
				scheduler.enqueueVirtualCluster(newObj)
			},
			DeleteFunc: scheduler.enqueueVirtualCluster,
		},
	)

	// Handle SuperCluster add&delete

	superInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: scheduler.enqueueSuperCluster,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newSuper := newObj.(*v1alpha4.Cluster)
				oldSuper := oldObj.(*v1alpha4.Cluster)
				if newSuper.ResourceVersion == oldSuper.ResourceVersion {
					return
				}
				scheduler.enqueueSuperCluster(newObj)
			},
			DeleteFunc: scheduler.enqueueSuperCluster,
		},
	)
	scheduler.virtualClusterLister = vcInformer.Lister()
	scheduler.virtualClusterSynced = vcInformer.Informer().HasSynced
	scheduler.superClusterLister = superInformer.Lister()
	scheduler.superClusterSynced = superInformer.Informer().HasSynced

	scheduler.schedulerCache = internalcache.NewSchedulerCache(stopCh)

	vcWatcher := manager.New()
	scheduler.virtualClusterWatcher = vcWatcher
	virtualClusterWatchers.Register(config, vcWatcher)

	superWatcher := manager.New()
	scheduler.superClusterWatcher = superWatcher
	// register super cluster resources
	initContext := &plugin.InitContext{Config: config}
	for _, p := range SuperClusterResourceRegister.List() {
		klog.Infof("loading super cluster resource plugin %q...", p.ID)

		result := p.Init(initContext)
		instance, err := result.Instance()
		if err != nil {
			klog.Errorf("failed to load plugin %q", p.ID)
			return nil, err
		}

		s, ok := instance.(manager.ResourceWatcher)
		if ok {
			scheduler.superClusterWatcher.AddResourceWatcher(s)
		} else {
			klog.Warningf("unrecognized super cluster resource plugin %q", p.ID)
		}
	}

	return scheduler, nil
}

func (s *Scheduler) enqueueVirtualCluster(obj interface{}) {
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
	s.virtualClusterQueue.Add(key)
}

func (s *Scheduler) enqueueSuperCluster(obj interface{}) {
	_, ok := obj.(*v1alpha4.Cluster)

	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("couldn't get object from tombstone %+v", obj))
			return
		}
		_, ok = tombstone.Obj.(*v1alpha4.Cluster)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object that is not a v1alpha4.cluster %+v", obj))
			return
		}
	}

	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	s.superClusterQueue.Add(key)
}

func (s *Scheduler) Run(stopChan <-chan struct{}) {

	if !cache.WaitForCacheSync(stopChan, s.virtualClusterSynced) {
		klog.Errorf("fail to sync virtualclustr informer cache")
		return
	}

	if !cache.WaitForCacheSync(stopChan, s.superClusterSynced) {
		klog.Errorf("fail to sync superclustr informer cache")
		return
	}

	if err := s.Bootstrap(); err != nil {
		klog.Errorf("initializing scheduler cache fails with error: %v", err)
		panic("the scheduler cannot start without an initialized cache")
	}

	go func() {
		if err := s.virtualClusterWatcher.Start(stopChan); err != nil {
			klog.Infof("virtualcluster watch manager exits: %v", err)
		}
	}()

	go func() {
		if err := s.superClusterWatcher.Start(stopChan); err != nil {
			klog.Infof("supercluster watch manager exits: %v", err)
		}
	}()

	go func() {
		defer utilruntime.HandleCrash()
		defer s.virtualClusterQueue.ShutDown()

		klog.Infof("starting scheduler virtualcluster workerqueue")
		defer klog.Infof("shutting down scheduler virtualcluster workerqueue")

		for i := 0; i < s.virtualClusterWorkers; i++ {
			go wait.Until(s.virtualClusterWorkerRun, 1*time.Second, stopChan)
		}
		<-stopChan
	}()

	go func() {
		defer utilruntime.HandleCrash()
		defer s.superClusterQueue.ShutDown()

		klog.Infof("starting scheduler supercluster workerqueue")
		defer klog.Infof("shutting down scheduler supercluster workerqueue")

		for i := 0; i < s.superClusterWorkers; i++ {
			go wait.Until(s.superClusterWorkerRun, 1*time.Second, stopChan)
		}
		<-stopChan

	}()
}

// The dirty sets are used in bootstrap and in handling cluster offline. If a cluster was in dirty set and becomes online again,
// the cluster state needs to be synchronized with the scheduler cache first during which the scheduler will not serve any scheduling
// request from that cluster.
var DirtyVirtualClusters sync.Map
var DirtySuperClusters sync.Map

// Bootstrap initializes the scheduler cache
func (s *Scheduler) Bootstrap() error {
	superList, err := s.superClusterLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list super cluster CRs: %v", err)
	}
	for _, each := range superList {
		if err := util.SyncSuperClusterState(s.metaClusterClient, each, s.schedulerCache); err != nil {
			key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(each)
			DirtySuperClusters.Store(key, struct{}{})
			// retry in super workerqueue
			s.enqueueSuperCluster(each)
		}
	}

	vcList, err := s.virtualClusterLister.List(labels.Everything())
	if err != nil {
		return fmt.Errorf("failed to list super cluster CRs: %v", err)
	}

	for _, each := range vcList {
		if err := util.SyncVirtualClusterState(s.metaClusterClient, each, s.schedulerCache); err != nil {
			key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(each)
			DirtyVirtualClusters.Store(key, struct{}{})
			// retry in vc workerqueue
			s.enqueueVirtualCluster(each)
		}
	}
	return nil
}
