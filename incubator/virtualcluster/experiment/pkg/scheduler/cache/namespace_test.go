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

func TestGetNumSlices(t *testing.T) {

	testcases := map[string]struct {
		quota      v1.ResourceList
		quotaSlice v1.ResourceList
		expect     int
		succeed    bool
	}{
		"Succeed with scale": {
			quota: v1.ResourceList{
				"cpu":    resource.MustParse("1000M"),
				"memory": resource.MustParse("2Gi"),
			},
			quotaSlice: v1.ResourceList{
				"cpu":    resource.MustParse("500M"),
				"memory": resource.MustParse("1Gi"),
			},
			expect:  2,
			succeed: true,
		},

		"Succeed with memory fragmentation": {
			quota: v1.ResourceList{
				"cpu":    resource.MustParse("1000M"),
				"memory": resource.MustParse("2Gi"),
			},
			quotaSlice: v1.ResourceList{
				"cpu":    resource.MustParse("10M"),
				"memory": resource.MustParse("1Gi"),
			},
			expect:  100,
			succeed: true,
		},

		"Succeed with cpu fragmentation": {
			quota: v1.ResourceList{
				"cpu":    resource.MustParse("1000M"),
				"memory": resource.MustParse("2Gi"),
			},
			quotaSlice: v1.ResourceList{
				"cpu":    resource.MustParse("200M"),
				"memory": resource.MustParse("50Mi"),
			},
			expect:  41,
			succeed: true,
		},

		"Succeed with both fragmentations": {
			quota: v1.ResourceList{
				"cpu":    resource.MustParse("1000M"),
				"memory": resource.MustParse("2Gi"),
			},
			quotaSlice: v1.ResourceList{
				"cpu":    resource.MustParse("800M"),
				"memory": resource.MustParse("800Mi"),
			},
			expect:  3,
			succeed: true,
		},

		"Fail with big slice": {
			quota: v1.ResourceList{
				"cpu":    resource.MustParse("1000M"),
				"memory": resource.MustParse("2Gi"),
			},
			quotaSlice: v1.ResourceList{
				"cpu":    resource.MustParse("8000M"),
				"memory": resource.MustParse("800Mi"),
			},
			expect:  0,
			succeed: false,
		},

		"Fail with missing resource in slice": {
			quota: v1.ResourceList{
				"cpu":    resource.MustParse("1000M"),
				"memory": resource.MustParse("2Gi"),
			},
			quotaSlice: v1.ResourceList{
				"cpu": resource.MustParse("8000M"),
			},
			expect:  0,
			succeed: false,
		},

		"Fail with more resource in slice": {
			quota: v1.ResourceList{
				"cpu":    resource.MustParse("1000M"),
				"memory": resource.MustParse("2Gi"),
			},
			quotaSlice: v1.ResourceList{
				"cpu":     resource.MustParse("8000M"),
				"memory":  resource.MustParse("800Mi"),
				"storage": resource.MustParse("1Gi"),
			},
			expect:  0,
			succeed: false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			num, err := GetNumSlices(tc.quota, tc.quotaSlice)
			if tc.succeed && err != nil {
				t.Errorf("test %s should succeed but fails with err %v", k, err)
			}

			if !tc.succeed && err == nil {
				t.Errorf("test %s should fail but succeeds", k)
			}

			if num != tc.expect {
				t.Errorf("The num is not expected. Exp: %v, Got %v", tc.expect, num)
			}

		})
	}

}
