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
	"strings"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/constants"
)

var _ Cache = &schedulerCache{}

type schedulerCache struct {
	stop <-chan struct{}

	mu sync.RWMutex

	tenants    map[string]struct{}
	clusters   map[string]*Cluster
	pods       map[string]*Pod
	namespaces map[string]*Namespace
}

func NewSchedulerCache(stop <-chan struct{}) Cache {
	c := &schedulerCache{
		stop:       stop,
		tenants:    make(map[string]struct{}),
		clusters:   make(map[string]*Cluster),
		pods:       make(map[string]*Pod),
		namespaces: make(map[string]*Namespace),
	}
	go wait.Until(c.GarbageCollection, 3*time.Minute, stop)
	return c
}

func (c *schedulerCache) GarbageCollection() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var csToDelete []*Cluster
	for _, v := range c.clusters {
		if v.shadow && metav1.Now().After(v.lastUpdateTime.Add(5*time.Minute)) {
			csToDelete = append(csToDelete, v)
		}
	}
	for _, each := range csToDelete {
		delete(c.clusters, each.name)
	}
}

func (c *schedulerCache) AddTenant(n string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tenants[n] = struct{}{}
}

func (c *schedulerCache) RemoveTenant(n string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var nsToDelete []*Namespace

	for _, v := range c.namespaces {
		if v.owner == n {
			nsToDelete = append(nsToDelete, v)
		}
	}

	var err error
	i := -1
	for _, each := range nsToDelete {
		err := c.removeNamespaceWithoutLock(each)
		if err != nil {
			break
		}
		i++
	}
	if err != nil {
		for ; i > -1; i-- {
			c.addNamespaceWithoutLock(nsToDelete[i])
		}
	} else {
		delete(c.tenants, n)
	}
	return err
}

func (c *schedulerCache) addPod(pod *Pod) error {
	cluster, ok := c.clusters[pod.cluster]
	if !ok {
		return fmt.Errorf("fail to add pod %s because cluster %s is not in the cache", pod.GetKey(), pod.cluster)
	}
	return cluster.AddPod(pod)
}

func (c *schedulerCache) AddPod(pod *Pod) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	clone := pod.DeepCopy()
	key := clone.GetKey()
	curState, ok := c.pods[key]
	if ok {
		if curState.cluster != clone.cluster {
			// Pod scheduling result is changed
			klog.Warningf("pod %s was added to cluster %s, but is adding to %s now", key, curState.cluster, clone.cluster)

			if err := c.removePod(curState); err != nil {
				return fmt.Errorf("fail to remove Pod %s with error %v", key, err)
			}
			if err := c.addPod(clone); err != nil {
				return fmt.Errorf("fail to add pod %s with error %v", key, err)
			}
		}
		c.pods[key] = clone
		return nil
	}

	if err := c.addPod(clone); err != nil {
		return fmt.Errorf("fail to add pod %s with error %v", key, err)
	}
	c.pods[key] = clone

	return nil
}

func (c *schedulerCache) removePod(pod *Pod) error {
	cluster, ok := c.clusters[pod.cluster]
	if !ok {
		return fmt.Errorf("fail to remove pod %s because cluster %s is not in the cache", pod.GetKey(), pod.cluster)
	}
	cluster.RemovePod(pod)
	return nil
}

func (c *schedulerCache) RemovePod(pod *Pod) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := pod.GetKey()

	curState, ok := c.pods[key]
	if !ok {
		return nil
	}

	if curState.cluster != pod.cluster {
		klog.Warningf("pod %s was added to cluster %s, but is adding to %s now, the cache is inconsistent", key, curState.cluster, pod.cluster)
	}
	if err := c.removePod(pod); err != nil {
		return fmt.Errorf("fail to remove pod %s with error %v", key, err)
	}
	delete(c.pods, key)

	return nil
}

func (c *schedulerCache) GetPod(key string) *Pod {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pods[key]
}

func (c *schedulerCache) GetNamespace(key string) *Namespace {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.namespaces[key]
}

func (c *schedulerCache) addNamespaceToCluster(cluster, key string, num int, slice v1.ResourceList) error {
	if num == 0 {
		return nil
	}
	clusterState, exists := c.clusters[cluster]
	if !exists {
		klog.Warningf("namespace %s has a placement to a cluster %s that does not exist, create a shadow cluster", key, cluster)
		clusterState = NewCluster(cluster, nil, constants.ShadowClusterCapacity)
		clusterState.shadow = true
	}
	var slices []*Slice
	for i := 0; i < num; i++ {
		slices = append(slices, NewSlice(key, slice, cluster))
	}
	if err := clusterState.AddNamespace(key, slices); err != nil {
		return err
	}
	if !exists {
		c.clusters[cluster] = clusterState
	}
	return nil
}

func (c *schedulerCache) AddNamespace(namespace *Namespace) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.tenants[namespace.owner]; !ok {
		return nil
	}

	key := namespace.GetKey()
	if old, ok := c.namespaces[key]; ok {
		return c.updateNamespaceWithoutLock(old, namespace)
	}
	return c.addNamespaceWithoutLock(namespace)
}

func (c *schedulerCache) addNamespaceWithoutLock(namespace *Namespace) error {
	clone := namespace.DeepCopy()
	key := clone.GetKey()
	if _, ok := c.namespaces[key]; ok {
		// Namespace update cannot be done in this method.
		return fmt.Errorf("namespace %s was added to cache", key)
	}

	expect, err := GetLeastFitSliceNum(clone.quota, clone.quotaSlice)
	if err != nil {
		return fmt.Errorf("fail to get the number of slices for namespace %s: %v", key, err)
	}

	sched := 0
	for _, each := range clone.schedule {
		sched = sched + each.num
	}
	if expect != sched {
		return fmt.Errorf("namespace %s has %d slices, but only %d have been scheduled, it cannot be added to cache", key, expect, sched)
	}
	i := -1

	for _, each := range clone.schedule {
		err = c.addNamespaceToCluster(each.cluster, key, each.num, clone.quotaSlice)
		if err != nil {
			break
		}
		i++
	}
	// We need to rollback if any error happens.
	if err != nil {
		for ; i > -1; i-- {
			// We don't expect any error here.
			c.removeNamespaceFromCluster(clone.schedule[i].cluster, key)
		}
	} else {
		c.namespaces[key] = clone
	}
	return err
}

func (c *schedulerCache) removeNamespaceFromCluster(cluster, key string) error {
	clusterState, ok := c.clusters[cluster]
	if !ok {
		return fmt.Errorf("namespace %s has a placement to a cluster %s that does not exist", key, cluster)
	}
	return clusterState.RemoveNamespace(key)
}

func (c *schedulerCache) RemoveNamespace(namespace *Namespace) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.removeNamespaceWithoutLock(namespace)
}

func (c *schedulerCache) removeNamespaceWithoutLock(namespace *Namespace) error {
	key := namespace.GetKey()
	if _, ok := c.namespaces[key]; !ok {
		return fmt.Errorf("namespace %s has been removed from the cache", key)
	}
	var err error
	i := -1
	for _, each := range namespace.schedule {
		err = c.removeNamespaceFromCluster(each.cluster, key)
		if err != nil {
			break
		}
		i++
	}

	// Rollback if any error happens.
	if err != nil {
		for ; i > -1; i-- {
			c.addNamespaceToCluster(namespace.schedule[i].cluster, key, namespace.schedule[i].num, namespace.quotaSlice)
		}
	} else {
		delete(c.namespaces, key)
	}

	return err
}

func (c *schedulerCache) updateNamespaceWithoutLock(oldNamespace, newNamespace *Namespace) error {
	var err error
	if oldNamespace.GetKey() != newNamespace.GetKey() {
		return fmt.Errorf("cannot update namespaces with different keys")
	}
	err = c.removeNamespaceWithoutLock(oldNamespace)
	if err != nil {
		return err
	}

	err = c.addNamespaceWithoutLock(newNamespace)
	if err != nil {
		c.addNamespaceWithoutLock(oldNamespace)
		return err
	}
	return nil
}

// UpdateNamespace handles changing namespace scheduling result.
// TODO: We need more detailed namespace update methods such as updating labels only.
func (c *schedulerCache) UpdateNamespace(oldNamespace, newNamespace *Namespace) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.tenants[oldNamespace.owner]; !ok {
		return nil
	}
	return c.updateNamespaceWithoutLock(oldNamespace, newNamespace)
}

func (c *schedulerCache) updateClusterNonAllocationStates(curCluster, newCluster *Cluster) {
	if newCluster.labels != nil {
		curCluster.labels = make(map[string]string)
		for k, v := range newCluster.labels {
			curCluster.labels[k] = v
		}
	}
	curCluster.capacity = newCluster.capacity.DeepCopy()
	curCluster.shadow = false

	provisionItemsCopy := make(map[string][]*Slice)
	for k, v := range newCluster.provisionItems {
		s := make([]*Slice, 0, len(v))
		for _, each := range v {
			s = append(s, each.DeepCopy())
		}
		provisionItemsCopy[k] = s
	}
	curCluster.provisionItems = provisionItemsCopy
	curCluster.provision = newCluster.provision.DeepCopy()
}

func (c *schedulerCache) AddCluster(cluster *Cluster) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if clusterState, ok := c.clusters[cluster.name]; ok {
		c.updateClusterNonAllocationStates(clusterState, cluster)
		return nil
	}
	clone := cluster.DeepCopy()
	c.clusters[cluster.name] = clone
	return nil
}

func (c *schedulerCache) RemoveCluster(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.clusters[name]; !ok {
		return fmt.Errorf("cluster %s was deleted from cache", name)
	}
	delete(c.clusters, name)
	return nil
}

func (c *schedulerCache) AddProvision(clustername, key string, slices []*Slice) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	clusterState, ok := c.clusters[clustername]
	if !ok {
		return fmt.Errorf("cluster %s is not in cache, cannot add provision for %s", clustername, key)
	}

	// clear old state if any
	if err := clusterState.RemoveProvision(key); err != nil {
		return err
	}
	return clusterState.AddProvision(key, slices)
}

func (c *schedulerCache) RemoveProvision(clustername, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	clusterState, ok := c.clusters[clustername]
	if !ok {
		return fmt.Errorf("cluster %s is not in cache, cannot remove provision for %s", clustername, key)
	}
	return clusterState.RemoveProvision(key)
}

func (c *schedulerCache) UpdateClusterCapacity(clustername string, newCapacity v1.ResourceList) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	clusterState, ok := c.clusters[clustername]
	if !ok {
		return fmt.Errorf("cluster %s is not in cache, cannot update the cluster capacity", clustername)
	}
	clusterState.capacity = newCapacity.DeepCopy()
	clusterState.lastUpdateTime = metav1.Now()
	return nil
}

func (c *schedulerCache) Dump() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := strings.Builder{}
	out.WriteString("-- Dump Super Clusters --")
	for k, v := range c.clusters {
		out.WriteByte('\n')
		out.WriteString(k)
		out.WriteByte(' ')
		out.WriteString(v.Dump())
	}

	out.WriteString("\n")
	out.WriteString("-- Dump Namespaces --")
	for k, v := range c.namespaces {
		out.WriteByte('\n')
		out.WriteString(k)
		out.WriteByte(' ')
		out.WriteString(v.Dump())
	}

	out.WriteString("\n")
	out.WriteString("-- Dump Pods --")
	for k, v := range c.pods {
		out.WriteByte('\n')
		out.WriteString(k)
		out.WriteByte(' ')
		out.WriteString(v.Dump())
	}
	return out.String()
}
