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
	"k8s.io/apimachinery/pkg/api/resource"
)

type Cluster struct {
	name     string
	labels   map[string]string
	capacity v1.ResourceList

	alloc  v1.ResourceList
	slices map[string][]*Slice            // ns key -> slice array
	pods   map[string]map[string]struct{} // ns key -> Pod map
}

func NewCluster(name string, labels map[string]string, capacity v1.ResourceList) *Cluster {
	alloc := capacity.DeepCopy()

	for k, v := range alloc {
		v.Set(0)
		alloc[k] = v
	}

	return &Cluster{
		name:     name,
		labels:   labels,
		capacity: capacity,
		alloc:    alloc,
		slices:   make(map[string][]*Slice),
		pods:     make(map[string]map[string]struct{}),
	}
}

func (c *Cluster) DeepCopy() *Cluster {
	labelcopy := make(map[string]string)
	for k, v := range c.labels {
		labelcopy[k] = v
	}

	out := NewCluster(c.name, c.labels, c.capacity.DeepCopy())

	slicesCopy := make(map[string][]*Slice)

	for k, v := range c.slices {
		s := make([]*Slice, 0, len(v))
		for _, each := range v {
			s = append(s, each.DeepCopy())
		}
		slicesCopy[k] = s
	}

	podsCopy := make(map[string]map[string]struct{})
	for k, v := range c.pods {
		podsCopy[k] = make(map[string]struct{})
		for name, _ := range v {
			podsCopy[k][name] = struct{}{}
		}
	}
	out.slices = slicesCopy
	out.pods = podsCopy
	out.alloc = c.alloc.DeepCopy()
	return out
}

func (c *Cluster) AddNamespace(key string, slices []*Slice) error {
	if _, ok := c.slices[key]; ok {
		return fmt.Errorf("namespace key %s is already in cluster %s, cannot add twice", key, c.name)
	}
	allocCopy := c.alloc.DeepCopy()
	for _, s := range slices {
		if s.cluster != c.name {
			return fmt.Errorf("slice %s is placed in cluster %s, not %s", s.owner, s.cluster, c.name)
		}
		for k, v := range s.size {
			each, ok := allocCopy[k]
			if !ok {
				return fmt.Errorf("slice %s has quota %s which is not in known cluster %s's allocable resources", s.owner, k, c.name)
			}
			each.Add(v)
			var upper resource.Quantity
			upper, ok = c.capacity[k]
			if !ok {
				return fmt.Errorf("slice %s has quota %s which is not in known cluster %s's capacity resources", s.owner, k, c.name)
			}

			if upper.Cmp(each) == -1 {
				return fmt.Errorf("cluster %s's resource %s allocation is > capacity after adding %s's slices ", c.name, k, key)
			}
			allocCopy[k] = each
		}
	}
	// apply changes only when no errors
	c.alloc = allocCopy
	c.slices[key] = slices
	return nil
}

func (c *Cluster) RemoveNamespace(key string) error {
	slices, ok := c.slices[key]
	if !ok {
		return fmt.Errorf("namespace key %s is not in cluster %s, cannot remove twice", key, c.name)
	}
	allocCopy := c.alloc.DeepCopy()
	for _, s := range slices {
		for k, v := range s.size {
			each, _ := allocCopy[k]
			if each.Cmp(v) == -1 {
				// This usually means the cache is messed up
				return fmt.Errorf("cluster %s's resource %s allocation is < 0 after removing %s's slices ", c.name, k, key)
			}
			each.Sub(v)
			allocCopy[k] = each
		}
	}
	c.alloc = allocCopy
	delete(c.slices, key)
	return nil
}

func (c *Cluster) AddPod(pod *Pod) error {
	key := pod.GetNamespaceKey()
	if pod.cluster != c.name {
		return fmt.Errorf("%s's Pod %s should not be placed in cluster %v (scheduled to %v) ", pod.GetNamespaceKey(), pod.name, c.name, pod.cluster)
	}
	if _, ok := c.pods[key]; !ok {
		c.pods[key] = make(map[string]struct{})
	}
	c.pods[key][pod.name] = struct{}{}
	return nil
}

func (c *Cluster) RemovePod(pod *Pod) {
	key := pod.GetNamespaceKey()
	delete(c.pods[key], pod.name)
	if len(c.pods[key]) == 0 {
		delete(c.pods, key)
	}
}
