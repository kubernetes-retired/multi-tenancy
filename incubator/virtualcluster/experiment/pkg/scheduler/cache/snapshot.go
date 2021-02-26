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

package cache

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
)

func MaxAlloc(a v1.ResourceList, b v1.ResourceList) v1.ResourceList {
	ret := a.DeepCopy()
	for key, value1 := range a {
		value2 := b[key]
		if value1.Cmp(value2) < 0 {
			ret[key] = value2.DeepCopy()
		}
	}
	return ret
}

type ClusterUsage struct {
	capacity  v1.ResourceList
	alloc     v1.ResourceList
	provision v1.ResourceList
}

func (u *ClusterUsage) GetCapacity() v1.ResourceList {
	return u.capacity
}

func (u *ClusterUsage) GetMaxAlloc() v1.ResourceList {
	return MaxAlloc(u.alloc, u.provision)
}

type NamespaceSchedSnapshot struct {
	clusterUsageMap map[string]*ClusterUsage
}

func (s *NamespaceSchedSnapshot) GetClusterUsageMap() map[string]*ClusterUsage {
	return s.clusterUsageMap
}

func (s *NamespaceSchedSnapshot) AddSlices(slices []*Slice) error {
	for _, each := range slices {
		cur, exists := s.clusterUsageMap[each.cluster]
		if !exists {
			return fmt.Errorf("slices are added to nonexistence cluster")
		}
		for k, v := range each.unit {
			val := cur.alloc[k].DeepCopy()
			val.Add(v)
			cur.alloc[k] = val
		}
	}
	return nil
}

func (s *NamespaceSchedSnapshot) RemoveSlices(slices []*Slice) error {
	for _, each := range slices {
		cur, exists := s.clusterUsageMap[each.cluster]
		if !exists {
			continue
		}
		for k, v := range each.unit {
			val := cur.alloc[k].DeepCopy()
			if val.Cmp(v) == -1 {
				return fmt.Errorf("slices removal causes negative allocation")
			}
			val.Sub(v)
			cur.alloc[k] = val
		}
	}
	return nil
}

func NewNamespaceSchedSnapshot() *NamespaceSchedSnapshot {
	return &NamespaceSchedSnapshot{
		clusterUsageMap: make(map[string]*ClusterUsage),
	}
}

func (c *schedulerCache) SnapshotForNamespaceSched(nsToRemove ...*Namespace) (*NamespaceSchedSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := NewNamespaceSchedSnapshot()
	for n, cluster := range c.clusters {
		if cluster.shadow {
			continue
		}
		s.clusterUsageMap[n] = &ClusterUsage{
			capacity:  cluster.capacity.DeepCopy(),
			alloc:     cluster.alloc.DeepCopy(),
			provision: cluster.provision.DeepCopy(),
		}
	}

	// in case of rescheduling, the old namespace needs to be removed from the snapshot
	for _, each := range nsToRemove {
		if each == nil {
			continue
		}
		curState, exists := c.namespaces[each.GetKey()]
		if !exists {
			continue
		}
		var slicesToRemove []*Slice
		for cluster, _ := range curState.GetPlacementMap() {
			if _, exists := s.clusterUsageMap[cluster]; !exists {
				continue
			}
			if _, exists := c.clusters[cluster].allocItems[each.GetKey()]; !exists {
				return nil, fmt.Errorf("fatal: cache is inconsistent")
			}
			slicesToRemove = append(slicesToRemove, c.clusters[cluster].allocItems[each.GetKey()]...)
		}
		if err := s.RemoveSlices(slicesToRemove); err != nil {
			return nil, err
		}
	}
	return s, nil
}

type PodSchedSnapshot struct {
	clusterUsageMap map[string]*ClusterUsage
}

func NewPodSchedSnapshot() *PodSchedSnapshot {
	return &PodSchedSnapshot{
		clusterUsageMap: make(map[string]*ClusterUsage),
	}
}

func (s *PodSchedSnapshot) GetClusterUsageMap() map[string]*ClusterUsage {
	return s.clusterUsageMap
}

func (s *PodSchedSnapshot) AddUsage(cluster string, usage v1.ResourceList) error {
	cur, exists := s.clusterUsageMap[cluster]
	if !exists {
		return fmt.Errorf("slices are added to nonexistence cluster")
	}
	for k, v := range usage {
		val := cur.alloc[k].DeepCopy()
		val.Add(v)
		cur.alloc[k] = val
	}
	return nil
}

func (c *schedulerCache) SnapshotForPodSched(pod *Pod) (*PodSchedSnapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := NewPodSchedSnapshot()
	nsKey := pod.GetNamespaceKey()

	ns := c.namespaces[nsKey]
	if ns == nil {
		return nil, fmt.Errorf("ns %s not found", nsKey)
	}

	for _, place := range ns.schedule {
		capability := v1.ResourceList{}
		alloc := v1.ResourceList{}

		for k, v := range ns.quotaSlice {
			val := v.DeepCopy()
			val.Set(v.Value() * int64(place.num))
			capability[k] = val
			val2 := v.DeepCopy()
			val2.Set(0)
			alloc[k] = val2
		}
		s.clusterUsageMap[place.cluster] = &ClusterUsage{
			capacity: capability,
			alloc:    alloc,
		}
	}

	// accumulate allocation for each pod
	for _, cluster := range c.clusters {
		for podName := range cluster.pods[nsKey] {
			podKey := fmt.Sprintf("%s/%s", nsKey, podName)

			// in case of rescheduling, the old pod needs to be removed from the snapshot
			if podKey == pod.GetKey() {
				continue
			}

			pod, ok := c.pods[podKey]
			if !ok {
				return nil, fmt.Errorf("cache is mess up, pod %s exists in ns but not in index", podKey)
			}
			rs, ok := s.clusterUsageMap[pod.cluster]
			if !ok {
				return nil, fmt.Errorf("cache is mess up, pod %s cluster %s is missing in ns %s", podKey, pod.cluster, nsKey)
			}

			for k, v := range pod.request {
				a, ok := rs.alloc[k]
				if !ok {
					continue
				}
				a.Add(v)
				rs.alloc[k] = a
			}
		}
	}

	return s, nil
}
