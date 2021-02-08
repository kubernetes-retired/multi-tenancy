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
	shadow   bool // a shadow cluster has a fake capacity, hence is not involved in scheduling

	alloc      v1.ResourceList
	allocItems map[string][]*Slice            // ns key -> slice array
	pods       map[string]map[string]struct{} // ns key -> Pod map

	// provision and provisionItems record the observed namespaces from the super cluster
	provision      v1.ResourceList
	provisionItems map[string][]*Slice
}

func NewCluster(name string, labels map[string]string, capacity v1.ResourceList) *Cluster {
	zeroRes := capacity.DeepCopy()

	for k, v := range zeroRes {
		v.Set(0)
		zeroRes[k] = v
	}

	return &Cluster{
		name:           name,
		labels:         labels,
		capacity:       capacity,
		alloc:          zeroRes,
		allocItems:     make(map[string][]*Slice),
		pods:           make(map[string]map[string]struct{}),
		provision:      zeroRes,
		provisionItems: make(map[string][]*Slice),
	}
}

func (c *Cluster) DeepCopy() *Cluster {
	var labelcopy map[string]string
	if c.labels != nil {
		labelcopy := make(map[string]string)
		for k, v := range c.labels {
			labelcopy[k] = v
		}
	}

	out := NewCluster(c.name, labelcopy, c.capacity.DeepCopy())

	allocItemsCopy := make(map[string][]*Slice)
	for k, v := range c.allocItems {
		s := make([]*Slice, 0, len(v))
		for _, each := range v {
			s = append(s, each.DeepCopy())
		}
		allocItemsCopy[k] = s
	}

	provisionItemsCopy := make(map[string][]*Slice)
	for k, v := range c.provisionItems {
		s := make([]*Slice, 0, len(v))
		for _, each := range v {
			s = append(s, each.DeepCopy())
		}
		provisionItemsCopy[k] = s
	}

	podsCopy := make(map[string]map[string]struct{})
	for k, v := range c.pods {
		podsCopy[k] = make(map[string]struct{})
		for name, _ := range v {
			podsCopy[k][name] = struct{}{}
		}
	}
	out.allocItems = allocItemsCopy
	out.alloc = c.alloc.DeepCopy()
	out.pods = podsCopy
	out.provision = c.provision.DeepCopy()
	out.provisionItems = provisionItemsCopy
	return out
}

func (c *Cluster) addItem(key string, items map[string][]*Slice, alloc v1.ResourceList, slices []*Slice) (v1.ResourceList, error) {
	if _, ok := items[key]; ok {
		return nil, fmt.Errorf("key %s is already in cluster %s, cannot add twice", key, c.name)
	}
	allocCopy := alloc.DeepCopy()
	for _, s := range slices {
		if s.cluster != c.name {
			return nil, fmt.Errorf("slice %s is placed in cluster %s, not %s", s.owner, s.cluster, c.name)
		}
		for k, v := range s.size {
			each, ok := allocCopy[k]
			if !ok {
				return nil, fmt.Errorf("slice %s has quota %s which is not in known cluster %s's allocable resources", s.owner, k, c.name)
			}
			each.Add(v)
			var upper resource.Quantity
			upper, ok = c.capacity[k]
			if !ok {
				return nil, fmt.Errorf("slice %s has quota %s which is not in known cluster %s's capacity resources", s.owner, k, c.name)
			}

			if upper.Cmp(each) == -1 {
				return nil, fmt.Errorf("cluster %s's resource %s allocation is > capacity after adding %s's allocItems ", c.name, k, key)
			}
			allocCopy[k] = each
		}
	}
	items[key] = slices
	return allocCopy, nil
}

func (c *Cluster) AddNamespace(key string, slices []*Slice) error {
	ret, err := c.addItem(key, c.allocItems, c.alloc, slices)
	if err == nil {
		c.alloc = ret
	}
	return err
}

func (c *Cluster) removeItem(key string, items map[string][]*Slice, alloc v1.ResourceList) (v1.ResourceList, error) {
	slices, ok := items[key]
	if !ok {
		return nil, fmt.Errorf("key %s is not in cluster %s, cannot remove twice", key, c.name)
	}
	allocCopy := alloc.DeepCopy()
	for _, s := range slices {
		for k, v := range s.size {
			each, _ := allocCopy[k]
			if each.Cmp(v) == -1 {
				// this usually means the cache is messed up
				return nil, fmt.Errorf("cluster %s's resource %s is < 0 after removing %s's slices ", c.name, k, key)
			}
			each.Sub(v)
			allocCopy[k] = each
		}
	}
	delete(items, key)
	return allocCopy, nil
}

func (c *Cluster) RemoveNamespace(key string) error {
	ret, err := c.removeItem(key, c.allocItems, c.alloc)
	if err == nil {
		c.alloc = ret
	}
	return err
}

func (c *Cluster) AddProvision(key string, slices []*Slice) error {
	ret, err := c.addItem(key, c.provisionItems, c.provision, slices)
	if err == nil {
		c.provision = ret
	}
	return err
}

func (c *Cluster) RemoveProvision(key string) error {
	ret, err := c.removeItem(key, c.provisionItems, c.provision)
	if err == nil {
		c.provision = ret
	}
	return err
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
