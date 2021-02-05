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

const (
	defaultTenant   = "tenant"
	defaultCluster1 = "testcluster1"
	defaultCluster2 = "testcluster2"
)

func TestAddRemoveNamespace(t *testing.T) {
	defaultCapacity := v1.ResourceList{
		"cpu":    resource.MustParse("2000M"),
		"memory": resource.MustParse("4Gi"),
	}

	fullQuota := v1.ResourceList{
		"cpu":    resource.MustParse("4000M"),
		"memory": resource.MustParse("8Gi"),
	}

	defaultQuota := v1.ResourceList{
		"cpu":    resource.MustParse("1000M"),
		"memory": resource.MustParse("2Gi"),
	}

	defaultQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("500M"),
		"memory": resource.MustParse("1Gi"),
	}

	stop := make(chan struct{})
	cache := NewSchedulerCache(stop)

	cluster1 := NewCluster(defaultCluster1, nil, defaultCapacity)
	cluster2 := NewCluster(defaultCluster2, nil, defaultCapacity)

	cache.AddCluster(cluster1)
	cache.AddCluster(cluster2)

	testcases := map[string]struct {
		namespace  *Namespace
		allocAfter map[string]v1.ResourceList
		succeed    bool
	}{
		"Succeed to add one namespace in two clusters": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 1),
					NewPlacement(defaultCluster2, 1),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
			},
			succeed: true,
		},

		"Succeed to add one namespace in one cluster": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 2),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("1000M"),
					"memory": resource.MustParse("2Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("0M"),
					"memory": resource.MustParse("0Gi"),
				},
			},
			succeed: true,
		},

		"Fail to add one namespace due to missing schedule": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 1),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("0M"),
					"memory": resource.MustParse("0Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("0M"),
					"memory": resource.MustParse("0Gi"),
				},
			},
			succeed: false,
		},

		"Succeed to add one namespace with shadow cluster": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 1),
					NewPlacement("shadow", 1),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("0M"),
					"memory": resource.MustParse("0Gi"),
				},
			},
			succeed: true,
		},

		"Fail to add one namespace due to wrong schedule": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, fullQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 2),
					NewPlacement(defaultCluster2, 6),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("0M"),
					"memory": resource.MustParse("0Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("0M"),
					"memory": resource.MustParse("0Gi"),
				},
			},
			succeed: false,
		},

		"Succeeed to add one namespace with full quota": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, fullQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 4),
					NewPlacement(defaultCluster2, 4),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("2000M"),
					"memory": resource.MustParse("4Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("2000M"),
					"memory": resource.MustParse("4Gi"),
				},
			},
			succeed: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			err := cache.AddNamespace(tc.namespace)
			if tc.succeed && err != nil {
				t.Errorf("test %s should succeed but fails with err %v", k, err)
			}
			if !tc.succeed && err == nil {
				t.Errorf("test %s should fail but succeeds", k)
			}
			if !Equals(tc.allocAfter[defaultCluster1], cache.clusters[defaultCluster1].alloc) {
				t.Errorf("The alloc of cluster 1 is not expected. Exp: %v, Got %v", tc.allocAfter[defaultCluster1], cache.clusters[defaultCluster1].alloc)
			}
			if !Equals(tc.allocAfter[defaultCluster2], cache.clusters[defaultCluster2].alloc) {
				t.Errorf("The alloc of cluster 2 is not expected. Exp: %v, Got %v", tc.allocAfter[defaultCluster2], cache.clusters[defaultCluster2].alloc)
			}
			cache.RemoveNamespace(tc.namespace)
		})

	}

}

func TestUpdateNamespace(t *testing.T) {
	defaultCapacity := v1.ResourceList{
		"cpu":    resource.MustParse("2000M"),
		"memory": resource.MustParse("4Gi"),
	}

	fullQuota := v1.ResourceList{
		"cpu":    resource.MustParse("4000M"),
		"memory": resource.MustParse("8Gi"),
	}

	defaultQuota := v1.ResourceList{
		"cpu":    resource.MustParse("1000M"),
		"memory": resource.MustParse("2Gi"),
	}

	defaultQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("500M"),
		"memory": resource.MustParse("1Gi"),
	}

	stop := make(chan struct{})
	cache := NewSchedulerCache(stop)

	cluster1 := NewCluster(defaultCluster1, nil, defaultCapacity)
	cluster2 := NewCluster(defaultCluster2, nil, defaultCapacity)

	cache.AddCluster(cluster1)
	cache.AddCluster(cluster2)

	testcases := map[string]struct {
		oldNamespace *Namespace
		newNamespace *Namespace
		allocAfter   map[string]v1.ResourceList
		succeed      bool
	}{
		"Succeed to update one namespace": {
			oldNamespace: NewNamespace(defaultTenant, defaultNamespace, nil, fullQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 4),
					NewPlacement(defaultCluster2, 4),
				}),
			newNamespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 1),
					NewPlacement(defaultCluster2, 1),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
			},
			succeed: true,
		},

		"Fail to update one namespace": {
			oldNamespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 1),
					NewPlacement(defaultCluster2, 1),
				}),
			newNamespace: NewNamespace(defaultTenant, defaultNamespace, nil, fullQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 6),
					NewPlacement(defaultCluster2, 2),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
			},
			succeed: false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			err := cache.AddNamespace(tc.oldNamespace)
			if err != nil {
				t.Errorf("test %s fail to create old namespace with err %v", k, err)
			}
			err = cache.UpdateNamespace(tc.oldNamespace, tc.newNamespace)
			if tc.succeed && err != nil {
				t.Errorf("test %s should succeed but fails with err %v", k, err)
			}
			if !tc.succeed && err == nil {
				t.Errorf("test %s should fail but succeeds", k)
			}
			if !Equals(tc.allocAfter[defaultCluster1], cache.clusters[defaultCluster1].alloc) {
				t.Errorf("The alloc of cluster 1 is not expected. Exp: %v, Got %v", tc.allocAfter[defaultCluster1], cache.clusters[defaultCluster1].alloc)
			}
			if !Equals(tc.allocAfter[defaultCluster2], cache.clusters[defaultCluster2].alloc) {
				t.Errorf("The alloc of cluster 2 is not expected. Exp: %v, Got %v", tc.allocAfter[defaultCluster2], cache.clusters[defaultCluster2].alloc)
			}
			cache.RemoveNamespace(tc.newNamespace)
		})

		// test AddNamespace interface as well
		t.Run(k, func(t *testing.T) {
			err := cache.AddNamespace(tc.oldNamespace)
			if err != nil {
				t.Errorf("test %s fail to create old namespace with err %v", k, err)
			}
			err = cache.AddNamespace(tc.newNamespace)
			if tc.succeed && err != nil {
				t.Errorf("test %s should succeed but fails with err %v", k, err)
			}
			if !tc.succeed && err == nil {
				t.Errorf("test %s should fail but succeeds", k)
			}
			if !Equals(tc.allocAfter[defaultCluster1], cache.clusters[defaultCluster1].alloc) {
				t.Errorf("The alloc of cluster 1 is not expected. Exp: %v, Got %v", tc.allocAfter[defaultCluster1], cache.clusters[defaultCluster1].alloc)
			}
			if !Equals(tc.allocAfter[defaultCluster2], cache.clusters[defaultCluster2].alloc) {
				t.Errorf("The alloc of cluster 2 is not expected. Exp: %v, Got %v", tc.allocAfter[defaultCluster2], cache.clusters[defaultCluster2].alloc)
			}
			cache.RemoveNamespace(tc.newNamespace)
		})
	}

}

func TestShadowCluster(t *testing.T) {
	defaultCapacity := v1.ResourceList{
		"cpu":    resource.MustParse("2000M"),
		"memory": resource.MustParse("4Gi"),
	}

	defaultQuota := v1.ResourceList{
		"cpu":    resource.MustParse("1000M"),
		"memory": resource.MustParse("2Gi"),
	}

	defaultQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("500M"),
		"memory": resource.MustParse("1Gi"),
	}

	stop := make(chan struct{})
	cache := NewSchedulerCache(stop)

	cluster1 := NewCluster(defaultCluster1, nil, defaultCapacity)
	cluster2 := NewCluster(defaultCluster2, nil, defaultCapacity)

	cache.AddCluster(cluster1)
	cache.AddCluster(cluster2)

	testcases := map[string]struct {
		namespace  *Namespace
		allocAfter map[string]v1.ResourceList
		succeed    bool
	}{
		"add namespace with shadowcluster": {
			namespace: NewNamespace(defaultTenant, defaultNamespace, nil, defaultQuota, defaultQuotaSlice,
				[]*Placement{
					NewPlacement(defaultCluster1, 1),
					NewPlacement("shadow", 1),
				}),
			allocAfter: map[string]v1.ResourceList{
				defaultCluster1: v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
				defaultCluster2: v1.ResourceList{
					"cpu":    resource.MustParse("0"),
					"memory": resource.MustParse("0"),
				},
				"shadow": v1.ResourceList{
					"cpu":    resource.MustParse("500M"),
					"memory": resource.MustParse("1Gi"),
				},
			},
			succeed: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			err := cache.AddNamespace(tc.namespace)
			if tc.succeed && err != nil {
				t.Errorf("test %s should succeed but fails with err %v", k, err)
			}
			if !tc.succeed && err == nil {
				t.Errorf("test %s should fail but succeeds", k)
			}
			if !Equals(tc.allocAfter[defaultCluster1], cache.clusters[defaultCluster1].alloc) {
				t.Errorf("The alloc of cluster 1 is not expected. Exp: %v, Got %v", tc.allocAfter[defaultCluster1], cache.clusters[defaultCluster1].alloc)

			}
			if !Equals(tc.allocAfter[defaultCluster2], cache.clusters[defaultCluster2].alloc) {
				t.Errorf("The alloc of cluster 2 is not expected. Exp: %v, Got %v", tc.allocAfter[defaultCluster2], cache.clusters[defaultCluster2].alloc)
			}

			if !Equals(tc.allocAfter["shadow"], cache.clusters["shadow"].alloc) {
				t.Errorf("The alloc of cluster shadow is not expected. Exp: %v, Got %v", tc.allocAfter["shadow"], cache.clusters["shadow"].alloc)
			}

			cluster3 := NewCluster("shadow", nil, defaultCapacity)
			if Equals(cluster3.capacity, cache.clusters["shadow"].capacity) {
				t.Errorf("The shadow cluster should have much bigger capacity. Got %v", cache.clusters["shadow"].capacity)
			}
			cache.AddCluster(cluster3)
			if !Equals(cluster3.capacity, cache.clusters["shadow"].capacity) || cache.clusters["shadow"].shadow {
				t.Errorf("The shadow cluster should have been reverted. Got %v", cache.clusters["shadow"].capacity)
			}
			if !Equals(tc.allocAfter["shadow"], cache.clusters["shadow"].alloc) {
				t.Errorf("The alloc of cluster shadow is not expected. Exp: %v, Got %v", tc.allocAfter["shadow"], cache.clusters["shadow"].alloc)
			}
		})
	}
}
