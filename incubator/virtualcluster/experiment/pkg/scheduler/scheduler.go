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

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	schedulerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/manager"
	//	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/reconciler"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/apis/cluster/v1alpha4"
	superclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/clientset/versioned"
	superinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/informers/externalversions/cluster/v1alpha4"
	superLister "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/listers/cluster/v1alpha4"
	superClusterWatchers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/resource/supercluster"
	virtualClusterWatchers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/resource/virtualcluster"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	virtualClusterLister "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

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
}

func New(
	config *schedulerconfig.SchedulerConfiguration,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	superClient superclient.Interface,
	superInformer superinformers.ClusterInformer,
	metaClusterClient clientset.Interface,
	metaInformers informers.SharedInformerFactory,
	recorder record.EventRecorder,
) *Scheduler {
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

	vcWatcher := manager.New()
	scheduler.virtualClusterWatcher = vcWatcher
	virtualClusterWatchers.Register(config, vcWatcher)

	superWatcher := manager.New()
	scheduler.superClusterWatcher = superWatcher
	superClusterWatchers.Register(config, superWatcher)

	return scheduler
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
	go func() {
		if err := s.virtualClusterWatcher.Start(stopChan); err != nil {
			klog.Infof("virtualcluster watch manager exits: %v", err)
		}
	}()

	go func() {
		defer utilruntime.HandleCrash()
		defer s.virtualClusterQueue.ShutDown()

		klog.Infof("starting scheduler virtualcluster workerqueue")
		defer klog.Infof("shutting down scheduler virtualcluster workerqueue")

		if !cache.WaitForCacheSync(stopChan, s.virtualClusterSynced) {
			return
		}

		klog.Infof("starting scheduler virtualcluster workers")
		for i := 0; i < s.virtualClusterWorkers; i++ {
			go wait.Until(s.virtualClusterWorkerRun, 1*time.Second, stopChan)
		}
		<-stopChan
	}()

	go func() {
		if err := s.superClusterWatcher.Start(stopChan); err != nil {
			klog.Infof("supercluster watch manager exits: %v", err)
		}
	}()

	go func() {
		defer utilruntime.HandleCrash()
		defer s.superClusterQueue.ShutDown()

		klog.Infof("starting scheduler supercluster workerqueue")
		defer klog.Infof("shutting down scheduler supercluster workerqueue")

		if !cache.WaitForCacheSync(stopChan, s.superClusterSynced) {
			return
		}

		klog.Infof("starting scheduler supercluster workers")
		for i := 0; i < s.superClusterWorkers; i++ {
			go wait.Until(s.superClusterWorkerRun, 1*time.Second, stopChan)
		}
		<-stopChan

	}()
}
