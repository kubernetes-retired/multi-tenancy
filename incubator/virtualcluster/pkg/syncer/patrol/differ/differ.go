/*
Copyright 2020 The Kubernetes Authors.

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

package differ

import (
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

type ClusterObject struct {
	metav1.Object
	Key          string
	OwnerCluster string
}

func (c ClusterObject) GetOwnerCluster() string {
	return c.OwnerCluster
}

type Handler interface {
	OnAdd(obj ClusterObject)
	OnDelete(obj ClusterObject)
	OnUpdate(obj1, obj2 ClusterObject)
}

type Differ interface {
	Insert(object ...ClusterObject)
	Has(object ClusterObject) bool
	Get(key string) ClusterObject
	Len() int
	Delete(object ClusterObject)
	Clear()
	GetKeys() sets.String
	// Difference compute the different keys between caller and callee.
	// receive a handler and execute to keep consistent with caller.
	Difference(Differ, Handler)
}

type container struct {
	mu  sync.Mutex
	set map[string]ClusterObject
}

func NewDiffSet(objects ...ClusterObject) Differ {
	c := &container{
		set: make(map[string]ClusterObject),
	}
	c.Insert(objects...)
	return c
}

func (c *container) Get(key string) ClusterObject {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.set[key]
}

func (c *container) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.set)
}

func (c *container) Insert(objects ...ClusterObject) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, object := range objects {
		c.set[object.Key] = object
	}
}

func (c *container) Has(object ClusterObject) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, contained := c.set[object.Key]
	return contained
}

func (c *container) GetKeys() sets.String {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := sets.NewString()
	for k := range c.set {
		s.Insert(k)
	}
	return s
}

func (c *container) Delete(object ClusterObject) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.set, object.Key)
}

func (c *container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.set = make(map[string]ClusterObject)
}

func (c *container) Difference(set2 Differ, handler Handler) {
	keySet1 := c.GetKeys()
	keySet2 := set2.GetKeys()

	groupedIntersectionSet := make(map[string]sets.String)
	for k, v := range c.set {
		if !keySet2.Has(k) {
			continue
		}
		keySet1.Delete(k)
		keySet2.Delete(k)
		group := v.OwnerCluster
		if group == "" {
			group = set2.Get(k).OwnerCluster
		}
		_, exists := groupedIntersectionSet[group]
		if !exists {
			groupedIntersectionSet[group] = sets.NewString()
		}
		groupedIntersectionSet[group].Insert(k)
	}

	var wg sync.WaitGroup

	for k := range keySet1 {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			handler.OnAdd(c.Get(key))
		}(k)
	}

	// the most possible case, concurrently processing them by cluster group
	// to avoid tons of go routines.
	for _, s := range groupedIntersectionSet {
		wg.Add(1)
		go func(iset sets.String) {
			defer wg.Done()
			for k := range iset {
				handler.OnUpdate(c.Get(k), set2.Get(k))
			}
		}(s)
	}

	for k := range keySet2 {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			handler.OnDelete(set2.Get(key))
		}(k)
	}

	wg.Wait()
}
