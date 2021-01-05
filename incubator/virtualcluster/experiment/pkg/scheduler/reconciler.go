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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

func (s *Scheduler) virtualClusterWorkerRun() {
	for s.processNextVirtualClusterItem() {
	}
}

func (s *Scheduler) processNextVirtualClusterItem() bool {
	key, quit := s.virtualClusterQueue.Get()
	if quit {
		return false
	}
	defer s.virtualClusterQueue.Done(key)

	err := s.syncVirtualCluster(key.(string))
	if err == nil {
		s.virtualClusterQueue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing virtual cluster %v (will retry): %v", key, err))
	s.virtualClusterQueue.AddRateLimited(key)
	return true
}

func (s *Scheduler) syncVirtualCluster(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	vc, err := s.virtualClusterLister.VirtualClusters(namespace).Get(name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		s.removeVirtualCluster(key)
		return nil
	}

	switch vc.Status.Phase {
	case v1alpha1.ClusterRunning:
		return s.addVirtualCluster(key, vc)
	case v1alpha1.ClusterError:
		s.removeVirtualCluster(key)
		return nil
	default:
		klog.Infof("Cluster %s/%s not ready to reconcile", vc.Namespace, vc.Name)
		return nil
	}
}

func (s *Scheduler) removeVirtualCluster(key string) {
	klog.Infof("Remove cluster %s", key)

	s.virtualClusterLock.Lock()
	defer s.virtualClusterLock.Unlock()

	vc, exist := s.virtualClusterSet[key]
	if !exist {
		// already deleted
		return
	}

	vc.Stop()

	for _, clusterChangeListener := range s.virtualClusterWatcher.GetListeners() {
		clusterChangeListener.RemoveCluster(vc)
	}

	delete(s.virtualClusterSet, key)
}

func (s *Scheduler) addVirtualCluster(key string, vc *v1alpha1.VirtualCluster) error {
	klog.Infof("Add cluster %s", key)

	s.virtualClusterLock.Lock()
	if _, exist := s.virtualClusterSet[key]; exist {
		s.virtualClusterLock.Unlock()
		return nil
	}
	s.virtualClusterLock.Unlock()

	clusterName := conversion.ToClusterKey(vc)

	adminKubeConfigBytes, err := conversion.GetKubeConfigOfVC(s.metaClusterClient.CoreV1(), vc)
	if err != nil {
		return err
	}

	tenantCluster, err := cluster.NewTenantCluster(clusterName, vc.Namespace, vc.Name, string(vc.UID), s.virtualClusterLister, adminKubeConfigBytes, cluster.Options{})
	if err != nil {
		return fmt.Errorf("failed to new tenant cluster %s/%s: %v", vc.Namespace, vc.Name, err)
	}

	// for each resource type of the newly added VirtualCluster, we add a listener
	for _, clusterChangeListener := range s.virtualClusterWatcher.GetListeners() {
		clusterChangeListener.AddCluster(tenantCluster)
	}

	s.virtualClusterLock.Lock()
	s.virtualClusterSet[key] = tenantCluster
	s.virtualClusterLock.Unlock()

	go s.syncVirtualClusterCache(tenantCluster, vc)

	return nil
}

func (s *Scheduler) syncVirtualClusterCache(cluster *cluster.Cluster, vc *v1alpha1.VirtualCluster) {
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
		s.removeVirtualCluster(key)
		s.virtualClusterQueue.AddAfter(key, 5*time.Second)
		return
	}
	cluster.SetSynced()
}
