/*
Copyright 2019 The Kubernetes Authors.

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

package conversion

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/utils/pointer"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
)

func TestCheckKVEquality(t *testing.T) {
	spec := v1alpha1.VirtualclusterSpec{
		TransparentMetaPrefixes: []string{"tp.x-k8s.io"},
		OpaqueMetaPrefixes:      []string{"tenancy.x-k8s.io"},
	}
	for _, tt := range []struct {
		name     string
		super    map[string]string
		virtual  map[string]string
		isEqual  bool
		expected map[string]string
	}{
		{
			name:     "both empty",
			super:    nil,
			virtual:  nil,
			isEqual:  true,
			expected: nil,
		},
		{
			name:  "empty super",
			super: nil,
			virtual: map[string]string{
				"a": "b",
			},
			isEqual: false,
			expected: map[string]string{
				"a": "b",
			},
		},
		{
			name: "equal",
			super: map[string]string{
				"a": "b",
			},
			virtual: map[string]string{
				"a": "b",
			},
			isEqual:  true,
			expected: nil,
		},
		{
			name: "not equal",
			super: map[string]string{
				"a": "b",
			},
			virtual: map[string]string{
				"b": "c",
				"a": "c",
			},
			isEqual: false,
			expected: map[string]string{
				"a": "c",
				"b": "c",
			},
		},
		{
			name: "less key",
			super: map[string]string{
				"a": "b",
				"b": "c",
			},
			virtual: map[string]string{
				"a": "c",
			},
			isEqual: false,
			expected: map[string]string{
				"a": "c",
			},
		},
		{
			name: "empty key",
			super: map[string]string{
				"a": "b",
				"b": "c",
			},
			virtual:  nil,
			isEqual:  false,
			expected: nil,
		},
		{
			name: "limiting key",
			super: map[string]string{
				"a": "b",
			},
			virtual: map[string]string{
				"a":                     "b",
				"tenancy.x-k8s.io/name": "name",
			},
			isEqual:  true,
			expected: nil,
		},
		{
			name: "limiting key and less key",
			super: map[string]string{
				"a":                     "b",
				"tenancy.x-k8s.io/name": "name",
			},
			virtual: nil,
			isEqual: false,
			expected: map[string]string{
				"tenancy.x-k8s.io/name": "name",
			},
		},
		{
			name: "ignore transparent key",
			super: map[string]string{
				"tenancy.x-k8s.io/name": "name",
				"tp.x-k8s.io/foo":       "val",
			},
			virtual:  nil,
			isEqual:  true,
			expected: nil,
		},
	} {
		t.Run(tt.name, func(tc *testing.T) {
			got, equal := Equality(&spec).checkDWKVEquality(tt.super, tt.virtual)
			if equal != tt.isEqual {
				tc.Errorf("expected equal %v, got %v", tt.isEqual, equal)
			} else {
				if !equality.Semantic.DeepEqual(got, tt.expected) {
					tc.Errorf("expected result %+v, got %+v", tt.expected, got)
				}
			}
		})
	}
}

func TestCheckContainersImageEquality(t *testing.T) {
	for _, tt := range []struct {
		name     string
		pObj     []v1.Container
		vObj     []v1.Container
		expected []v1.Container
	}{
		{
			name: "equal",
			pObj: []v1.Container{
				{
					Name:  "c1",
					Image: "image1",
				},
				{
					Name:  "c2",
					Image: "image2",
				},
			},
			vObj: []v1.Container{
				{
					Name:  "c1",
					Image: "image1",
				},
				{
					Name:  "c2",
					Image: "image2",
				},
			},
			expected: nil,
		},
		{
			name: "not equal",
			pObj: []v1.Container{
				{
					Name:  "c1",
					Image: "image1",
				},
				{
					Name:  "c2",
					Image: "image2",
				},
			},
			vObj: []v1.Container{
				{
					Name:  "c1",
					Image: "image1",
				},
				{
					Name:  "c2",
					Image: "image3",
				},
			},
			expected: []v1.Container{
				{
					Name:  "c1",
					Image: "image1",
				},
				{
					Name:  "c2",
					Image: "image3",
				},
			},
		},
	} {
		t.Run(tt.name, func(tc *testing.T) {
			got := Equality(nil).checkContainersImageEquality(tt.pObj, tt.vObj)
			if !equality.Semantic.DeepEqual(got, tt.expected) {
				t.Errorf("expected %+v, got %+v", tt.expected, got)
			}
		})
	}
}

func TestCheckActiveDeadlineSecondsEquality(t *testing.T) {
	for _, tt := range []struct {
		name       string
		pObj       *int64
		vObj       *int64
		isEqual    bool
		updatedVal *int64
	}{
		{
			name:       "both nil",
			pObj:       nil,
			vObj:       nil,
			isEqual:    true,
			updatedVal: nil,
		},
		{
			name:       "both not nil and equal",
			pObj:       pointer.Int64Ptr(1),
			vObj:       pointer.Int64Ptr(1),
			isEqual:    true,
			updatedVal: nil,
		},
		{
			name:       "both not nil but not equal",
			pObj:       pointer.Int64Ptr(1),
			vObj:       pointer.Int64Ptr(2),
			isEqual:    false,
			updatedVal: pointer.Int64Ptr(2),
		},
		{
			name:       "updated to nil",
			pObj:       pointer.Int64Ptr(1),
			vObj:       nil,
			isEqual:    false,
			updatedVal: nil,
		},
		{
			name:       "updated to value",
			pObj:       nil,
			vObj:       pointer.Int64Ptr(1),
			isEqual:    false,
			updatedVal: pointer.Int64Ptr(1),
		},
	} {
		t.Run(tt.name, func(tc *testing.T) {
			val, equal := Equality(nil).checkInt64Equality(tt.pObj, tt.vObj)
			if equal != tt.isEqual {
				tc.Errorf("expected equal %v, got %v", tt.isEqual, equal)
			}
			if !equal {
				if !equality.Semantic.DeepEqual(val, tt.updatedVal) {
					tc.Errorf("expected val %v, got %v", tt.updatedVal, val)
				}
			}
		})
	}
}
