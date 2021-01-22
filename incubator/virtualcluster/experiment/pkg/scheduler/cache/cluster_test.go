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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"k8s.io/apimachinery/pkg/api/equality"
)

const (
	defaultNamespace = "testnamespace"
	defaultCluster   = "testcluster"
)

func Equals(a v1.ResourceList, b v1.ResourceList) bool {
	if len(a) != len(b) {
		return false
	}

	for key, value1 := range a {
		value2, found := b[key]
		if !found {
			return false
		}
		if value1.Cmp(value2) != 0 {
			return false
		}
	}

	return true
}

func TestAddNamespace(t *testing.T) {
	defaultCapacity := v1.ResourceList{
		"cpu":    resource.MustParse("2000M"),
		"memory": resource.MustParse("4Gi"),
	}

	defaultQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("500M"),
		"memory": resource.MustParse("1Gi"),
	}

	overMemQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("400M"),
		"memory": resource.MustParse("5Gi"),
	}

	overCpuQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("4000M"),
		"memory": resource.MustParse("3Gi"),
	}

	testcases := map[string]struct {
		cluster    *Cluster
		slices     []*Slice
		allocAfter v1.ResourceList
		succeed    bool
	}{
		"Succeed to add one slice": {
			cluster: NewCluster(defaultCluster, nil, defaultCapacity),
			slices: []*Slice{
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
			},
			allocAfter: v1.ResourceList{
				"cpu":    resource.MustParse("500M"),
				"memory": resource.MustParse("1Gi"),
			},
			succeed: true,
		},

		"Succeed to add two slices": {
			cluster: NewCluster(defaultCluster, nil, defaultCapacity),
			slices: []*Slice{
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
			},
			allocAfter: v1.ResourceList{
				"cpu":    resource.MustParse("1000M"),
				"memory": resource.MustParse("2Gi"),
			},
			succeed: true,
		},

		"Fail to due to exceeding cluster memory capacity": {
			cluster: NewCluster(defaultCluster, nil, defaultCapacity),
			slices: []*Slice{
				NewSlice(defaultNamespace, overMemQuotaSlice, defaultCluster),
			},
			allocAfter: v1.ResourceList{
				"cpu":    resource.MustParse("0M"),
				"memory": resource.MustParse("0Gi"),
			},
			succeed: false,
		},

		"Fail due to exceeding cluster cpu capacity": {
			cluster: NewCluster(defaultCluster, nil, defaultCapacity),
			slices: []*Slice{
				NewSlice(defaultNamespace, overCpuQuotaSlice, defaultCluster),
			},
			allocAfter: v1.ResourceList{
				"cpu":    resource.MustParse("0M"),
				"memory": resource.MustParse("0Gi"),
			},
			succeed: false,
		},

		"Fail to add due to exceeding capacity with multiple slices": {
			cluster: NewCluster(defaultCluster, nil, defaultCapacity),
			slices: []*Slice{
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
			},
			allocAfter: v1.ResourceList{
				"cpu":    resource.MustParse("0M"),
				"memory": resource.MustParse("0Gi"),
			},
			succeed: false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			err := tc.cluster.AddNamespace(defaultNamespace, tc.slices)
			if tc.succeed && err != nil {
				t.Errorf("test %s should succeed but fails with err %v", k, err)
			}

			if !tc.succeed && err == nil {
				t.Errorf("test %s should fail but succeeds", k)
			}

			if !Equals(tc.allocAfter, tc.cluster.alloc) {
				t.Errorf("the alloc is not expected. Exp: %v, Got %v", tc.allocAfter, tc.cluster.alloc)
			}
		})

	}

}

func TestRemoveNamespace(t *testing.T) {
	defaultCapacity := v1.ResourceList{
		"cpu":    resource.MustParse("2000M"),
		"memory": resource.MustParse("4Gi"),
	}

	defaultQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("500M"),
		"memory": resource.MustParse("1Gi"),
	}

	defaultAlloc := v1.ResourceList{
		"cpu":    resource.MustParse("1000M"),
		"memory": resource.MustParse("2Gi"),
	}

	testcases := map[string]struct {
		cluster    *Cluster
		curSlices  []*Slice
		curAlloc   v1.ResourceList
		allocAfter v1.ResourceList
		succeed    bool
	}{
		"Succeed to remove one slice": {
			cluster: NewCluster(defaultCluster, nil, defaultCapacity),
			curSlices: []*Slice{
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
			},
			curAlloc: defaultAlloc,
			allocAfter: v1.ResourceList{
				"cpu":    resource.MustParse("500M"),
				"memory": resource.MustParse("1Gi"),
			},
			succeed: true,
		},

		"Succeed to remove two slices": {
			cluster: NewCluster(defaultCluster, nil, defaultCapacity),
			curSlices: []*Slice{
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
			},
			curAlloc: defaultAlloc,
			allocAfter: v1.ResourceList{
				"cpu":    resource.MustParse("0M"),
				"memory": resource.MustParse("0Gi"),
			},
			succeed: true,
		},

		"Fail due to cache mess up": {
			cluster: NewCluster(defaultCluster, nil, defaultCapacity),
			curSlices: []*Slice{
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
				NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster),
			},
			curAlloc:   defaultAlloc,
			allocAfter: defaultAlloc,
			succeed:    false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tc.cluster.alloc = tc.curAlloc
			for _, each := range tc.curSlices {
				tc.cluster.slices[defaultNamespace] = append(tc.cluster.slices[defaultNamespace], each)
			}

			err := tc.cluster.RemoveNamespace(defaultNamespace)
			if tc.succeed && err != nil {
				t.Errorf("test %s should succeed but fails with err %v", k, err)
			}

			if !tc.succeed && err == nil {
				t.Errorf("test %s should fail but succeeds", k)
			}

			if !Equals(tc.allocAfter, tc.cluster.alloc) {
				t.Errorf("the alloc is not expected. Exp: %v, Got %v", tc.allocAfter, tc.cluster.alloc)
			}
		})

	}

}

func TestDeepCopy(t *testing.T) {
	defaultCapacity := v1.ResourceList{
		"cpu":    resource.MustParse("2000M"),
		"memory": resource.MustParse("4Gi"),
	}
	defaultRequest := v1.ResourceList{
		"cpu":    resource.MustParse("1000M"),
		"memory": resource.MustParse("2Gi"),
	}
	defaultQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("500M"),
		"memory": resource.MustParse("1Gi"),
	}

	cluster := NewCluster(defaultCluster, nil, defaultCapacity)
	pod := NewPod("tenant", defaultNamespace, "pod-1", "123456", defaultCluster, defaultRequest)

	cluster.AddPod(pod)
	slices := []*Slice{NewSlice(defaultNamespace, defaultQuotaSlice, defaultCluster)}
	cluster.AddNamespace(defaultNamespace, slices)
	clone := cluster.DeepCopy()

	if clone.name != cluster.name ||
		!equality.Semantic.DeepEqual(clone.capacity, cluster.capacity) ||
		!equality.Semantic.DeepEqual(clone.alloc, cluster.alloc) ||
		clone.slices[defaultNamespace][0].owner != cluster.slices[defaultNamespace][0].owner ||
		!equality.Semantic.DeepEqual(clone.slices[defaultNamespace][0].size, cluster.slices[defaultNamespace][0].size) ||
		clone.slices[defaultNamespace][0].cluster != cluster.slices[defaultNamespace][0].cluster ||
		!equality.Semantic.DeepEqual(clone.pods[pod.GetNamespaceKey()], cluster.pods[pod.GetNamespaceKey()]) {
		t.Errorf("deepcopy fails %v %v", clone.alloc, cluster.alloc)
	}
}
