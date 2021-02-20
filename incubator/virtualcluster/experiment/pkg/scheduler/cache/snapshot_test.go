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
)

func TestSnapshotForNamespaceSched(t *testing.T) {
	defaultCapacity := v1.ResourceList{
		"cpu":    resource.MustParse("4"),
		"memory": resource.MustParse("8Gi"),
	}

	defaultQuota := v1.ResourceList{
		"cpu":    resource.MustParse("2"),
		"memory": resource.MustParse("4Gi"),
	}

	defaultQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("0.5"),
		"memory": resource.MustParse("1Gi"),
	}

	stop := make(chan struct{})
	cache := NewSchedulerCache(stop).(*schedulerCache)

	cluster1 := NewCluster(defaultCluster1, nil, defaultCapacity)
	cluster2 := NewCluster(defaultCluster2, nil, defaultCapacity)

	cache.AddCluster(cluster1)
	cache.AddCluster(cluster2)

	testcases := map[string]struct {
		namespace    *Namespace
		remove       bool
		provision    map[string]v1.ResourceList
		snapAllocMax map[string]v1.ResourceList
	}{
		"Snapshot with one namespace, two clusters": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 3),
					NewPlacement(defaultCluster2, 1),
				}),
			remove: false,
			provision: map[string]v1.ResourceList{
				defaultCluster1: nil,
				defaultCluster2: nil,
			},
			snapAllocMax: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("1.5"),
					"memory": resource.MustParse("3Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("0.5"),
					"memory": resource.MustParse("1Gi"),
				},
			},
		},

		"Snapshot with one namespace, and a shadow cluster": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 2),
					NewPlacement("shadow", 2),
				}),
			remove: false,
			provision: map[string]v1.ResourceList{
				defaultCluster1: nil,
				defaultCluster2: nil,
			},
			snapAllocMax: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("1"),
					"memory": resource.MustParse("2Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("0"),
					"memory": resource.MustParse("0"),
				},
			},
		},

		"Snapshot with one namespace, and a pre-provisioned cluster": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 2),
					NewPlacement(defaultCluster2, 2),
				}),
			remove: false,
			provision: map[string]v1.ResourceList{
				defaultCluster1: nil,
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("4"),
					"memory": resource.MustParse("1Gi"),
				},
			},
			snapAllocMax: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("1"),
					"memory": resource.MustParse("2Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("4"),
					"memory": resource.MustParse("2Gi"),
				},
			},
		},

		"Snapshot with one namespace (to be removed), and a provisioned cluster": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 2),
					NewPlacement(defaultCluster2, 2),
				}),
			remove: true,
			provision: map[string]v1.ResourceList{
				defaultCluster1: nil,
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("4"),
					"memory": resource.MustParse("1Gi"),
				},
			},
			snapAllocMax: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("0"),
					"memory": resource.MustParse("0"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("4"),
					"memory": resource.MustParse("1Gi"),
				},
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			cache.AddNamespace(tc.namespace)
			cache.clusters[defaultCluster1].provision = tc.provision[defaultCluster1]
			cache.clusters[defaultCluster2].provision = tc.provision[defaultCluster2]
			var s *NamespaceSchedSnapshot
			if !tc.remove {
				s, _ = cache.SnapshotForNamespaceSched()
			} else {
				s, _ = cache.SnapshotForNamespaceSched(tc.namespace)
			}

			max1 := MaxAlloc(s.clusterUsageMap[defaultCluster1].alloc, s.clusterUsageMap[defaultCluster1].provision)
			max2 := MaxAlloc(s.clusterUsageMap[defaultCluster2].alloc, s.clusterUsageMap[defaultCluster2].provision)

			if !Equals(max1, tc.snapAllocMax[defaultCluster1]) || !Equals(max2, tc.snapAllocMax[defaultCluster2]) {
				t.Errorf("snapshot is wrong. Exp: %v, Got %v %v", tc.snapAllocMax, max1, max2)
			}

			if _, exists := cache.clusters["shadow"]; exists {
				if _, exists := s.clusterUsageMap["shadow"]; exists {
					t.Errorf("shadow cluster should not be in the snapshot")
				}
			}
			cache.RemoveNamespace(tc.namespace)
			cache.clusters[defaultCluster1].provision = nil
			cache.clusters[defaultCluster2].provision = nil
		})

	}

}
