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

package engine

import (
	"reflect"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/algorithm"
	internalcache "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/cache"
)

func classifySlicesInfoArray(in algorithm.SliceInfoArray) (map[string]int, map[string]int, int) {
	mandatory := make(map[string]int)
	hint := make(map[string]int)
	regular := 0
	for _, each := range in {
		if each.Mandatory != "" {
			mandatory[each.Mandatory] = mandatory[each.Mandatory] + 1
		} else if each.Hint != "" {
			hint[each.Hint] = hint[each.Hint] + 1
		} else {
			regular++
		}
	}
	return mandatory, hint, regular
}

func TestGetSlicesToSchedule(t *testing.T) {

	defaultQuota := v1.ResourceList{
		"cpu":    resource.MustParse("10"),
		"memory": resource.MustParse("10Gi"),
	}

	defaultQuotaSlice := v1.ResourceList{
		"cpu":    resource.MustParse("1"),
		"memory": resource.MustParse("1Gi"),
	}

	namespace := internalcache.NewNamespace("testcluster", "testnamespace", nil, defaultQuota, defaultQuotaSlice, nil)

	testcases := map[string]struct {
		mandatoryPlacements map[string]int
		oldPlacements       map[string]int
		mandatory           map[string]int
		hint                map[string]int
		regular             int
	}{
		"all regualr": {
			mandatoryPlacements: map[string]int{},
			oldPlacements:       map[string]int{},
			mandatory:           map[string]int{},
			hint:                map[string]int{},
			regular:             10,
		},
		"a few mandatory (increase quota)": {
			mandatoryPlacements: map[string]int{
				"a": 2,
				"b": 4,
			},
			oldPlacements: map[string]int{},
			mandatory: map[string]int{
				"a": 2,
				"b": 4,
			},
			hint:    map[string]int{},
			regular: 4,
		},
		"more mandatory (increase quota)": {
			mandatoryPlacements: map[string]int{
				"a": 20,
			},
			oldPlacements: map[string]int{},
			mandatory: map[string]int{
				"a": 10,
			},
			hint:    map[string]int{},
			regular: 0,
		},
		"a few hints (increase quota)": {
			mandatoryPlacements: map[string]int{},
			oldPlacements: map[string]int{
				"c": 1,
				"d": 3,
			},
			mandatory: map[string]int{},
			hint: map[string]int{
				"c": 1,
				"d": 3,
			},
			regular: 6,
		},
		"more hints (decrease quota)": {
			mandatoryPlacements: map[string]int{},
			oldPlacements: map[string]int{
				"c": 13,
			},
			mandatory: map[string]int{},
			hint: map[string]int{
				"c": 10,
			},
			regular: 0,
		},
		"Mix mandatory and hints": {
			mandatoryPlacements: map[string]int{
				"a": 1,
				"b": 2,
			},
			oldPlacements: map[string]int{
				"c": 1,
				"d": 3,
			},
			mandatory: map[string]int{
				"a": 1,
				"b": 2,
			},
			hint: map[string]int{
				"c": 1,
				"d": 3,
			},
			regular: 3,
		},
		"Mix mandatory and hints - partial overlapping": {
			mandatoryPlacements: map[string]int{
				"a": 1,
				"b": 2,
			},
			oldPlacements: map[string]int{
				"a": 1,
				"d": 3,
			},
			mandatory: map[string]int{
				"a": 1,
				"b": 2,
			},
			hint: map[string]int{
				"d": 3,
			},
			regular: 4,
		},
		"Mix mandatory and hints - full overlapping": {
			mandatoryPlacements: map[string]int{
				"a": 1,
				"b": 2,
			},
			oldPlacements: map[string]int{
				"a": 3,
				"b": 4,
			},
			mandatory: map[string]int{
				"a": 1,
				"b": 2,
			},
			hint: map[string]int{
				"a": 2,
				"b": 2,
			},
			regular: 3,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			namespace.SetNewPlacements(tc.mandatoryPlacements)
			slicesToSchedule := GetSlicesToSchedule(namespace, tc.oldPlacements)
			mandatory, hint, regular := classifySlicesInfoArray(slicesToSchedule)
			if !reflect.DeepEqual(mandatory, tc.mandatory) || !reflect.DeepEqual(hint, tc.hint) || regular != tc.regular {
				t.Errorf("test %s should succeed but fails: %v(%v), %v(%v), %d(%d)", k, mandatory, tc.mandatory, hint, tc.hint, regular, tc.regular)
			}
		})
	}

}
