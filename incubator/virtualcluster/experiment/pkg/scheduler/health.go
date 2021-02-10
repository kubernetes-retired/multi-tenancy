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
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/util"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

var (
	numHealthSuperCluster     uint64
	numUnHealthSuperCluster   uint64
	numHealthVirtualCluster   uint64
	numUnHealthVirtualCluster uint64
)

func (s *Scheduler) superClusterHealthPatrol() {
	var clusters []mc.ClusterInterface
	s.superClusterLock.Lock()
	for _, c := range s.superClusterSet {
		clusters = append(clusters, c)
	}
	s.superClusterLock.Unlock()

	numUnHealthSuperCluster = 0
	numHealthSuperCluster = 0

	if len(clusters) != 0 {
		wg := sync.WaitGroup{}
		for _, c := range clusters {
			wg.Add(1)
			go func(cluster mc.ClusterInterface) {
				defer wg.Done()
				s.checkSuperClusterHealth(cluster)
			}(c)
		}
		wg.Wait()
	}

	metrics.SuperClusterHealthStats.WithLabelValues("health").Set(float64(numHealthSuperCluster))
	metrics.SuperClusterHealthStats.WithLabelValues("unhealth").Set(float64(numUnHealthSuperCluster))
}

// checkSuperClusterHealth attempts to get the node list from the super cluster, it will update the scheduler cache as well
func (s *Scheduler) checkSuperClusterHealth(cluster mc.ClusterInterface) {
	cs, err := cluster.GetClientSet()
	if err != nil {
		klog.Warningf("[checkSuperClusterHealth] fails to get cluster %v clientset: %v", cluster.GetClusterName(), err)
		return
	}

	var capacity v1.ResourceList
	capacity, err = util.GetSuperClusterCapacity(cs)
	if err != nil {
		klog.Warningf("[checkSuperClusterHealth] fails to get cluster %v capacity: %v", cluster.GetClusterName(), err)
		atomic.AddUint64(&numUnHealthSuperCluster, 1)

		ns, name, uid := cluster.GetOwnerInfo()
		s.recorder.Eventf(&v1.ObjectReference{
			Kind:      "Cluster",
			Namespace: ns,
			Name:      name,
			UID:       types.UID(uid),
		}, v1.EventTypeWarning, "ClusterUnHealth", "SuperCluster %v unhealth: %v", cluster.GetClusterName(), err.Error())

		// mark super cluster dirty and add to super cluster queue to resync the cache
		key := fmt.Sprintf("%s/%s", ns, name)
		DirtySuperClusters.Store(key, struct{}{})
		s.superClusterQueue.Add(key)
		return
	}
	atomic.AddUint64(&numHealthSuperCluster, 1)
	// update scheduler cache
	s.schedulerCache.UpdateClusterCapacity(cluster.GetClusterName(), capacity)
}

func (s *Scheduler) virtualClusterHealthPatrol() {
	var clusters []mc.ClusterInterface
	s.virtualClusterLock.Lock()
	for _, c := range s.virtualClusterSet {
		clusters = append(clusters, c)
	}
	s.virtualClusterLock.Unlock()

	numUnHealthVirtualCluster = 0
	numHealthVirtualCluster = 0

	if len(clusters) != 0 {
		wg := sync.WaitGroup{}
		for _, c := range clusters {
			wg.Add(1)
			go func(cluster mc.ClusterInterface) {
				defer wg.Done()
				s.checkVirtualClusterHealth(cluster)
			}(c)
		}
		wg.Wait()
	}

	metrics.VirtualClusterHealthStats.WithLabelValues("health").Set(float64(numHealthVirtualCluster))
	metrics.VirtualClusterHealthStats.WithLabelValues("unhealth").Set(float64(numUnHealthVirtualCluster))
}

func (s *Scheduler) checkVirtualClusterHealth(cluster mc.ClusterInterface) {
	cs, err := cluster.GetClientSet()
	if err != nil {
		klog.Warningf("[checkVirtualClusterHealth] fails to get cluster %v clientset: %v", cluster.GetClusterName(), err)
		return
	}

	_, err = cs.Discovery().ServerVersion()
	if err != nil {
		klog.Warningf("[checkVirtualClusterHealth] fails to get cluster %v version: %v", cluster.GetClusterName(), err)
		atomic.AddUint64(&numUnHealthVirtualCluster, 1)

		ns, name, uid := cluster.GetOwnerInfo()
		s.recorder.Eventf(&v1.ObjectReference{
			Kind:      "VirtualCluster",
			Namespace: ns,
			Name:      name,
			UID:       types.UID(uid),
		}, v1.EventTypeWarning, "ClusterUnHealth", "VirtualCluster %v unhealth: %v", cluster.GetClusterName(), err.Error())

		// mark virtual cluster dirty and add to virtual cluster queue to resync the cache
		key := fmt.Sprintf("%s/%s", ns, name)
		DirtyVirtualClusters.Store(key, struct{}{})
		s.virtualClusterQueue.Add(key)
		return
	}
	atomic.AddUint64(&numHealthVirtualCluster, 1)
}
