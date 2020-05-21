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

package storageclass

import (
	"testing"

	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func TestStorageClassPatrol(t *testing.T) {
	testTenant := &v1alpha1.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "tenant-1",
			UID:       "7374a172-c35d-45b1-9c8e-bf5c5b614937",
		},
		Spec: v1alpha1.VirtualClusterSpec{},
		Status: v1alpha1.VirtualClusterStatus{
			Phase: v1alpha1.ClusterRunning,
		},
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedDeletedVObject []string
		ExpectedCreatedVObject []string
		ExpectedUpdatedVObject []runtime.Object
		ExpectedNoOperation    bool
		WaitDWS                bool // Make sure to set this flag if the test involves DWS.
		WaitUWS                bool // Make sure to set this flag if the test involves UWS.
	}{
		"pStorageClass not public": {
			ExistingObjectInSuper: []runtime.Object{
				makeStorageClass("sc", "12345"),
			},
			ExpectedNoOperation: true,
		},
		"pStorageClass exists, vStorageClass does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				makeStorageClass("sc", "12345", func(class *v1.StorageClass) {
					class.Labels = map[string]string{
						constants.PublicObjectKey: "true",
					}
				}),
			},
			WaitUWS: true,
			ExpectedCreatedVObject: []string{
				"sc",
			},
		},
		"pStorageClass not found, vStorageClass exists": {
			ExistingObjectInTenant: []runtime.Object{
				makeStorageClass("sc", "12345"),
			},
			ExpectedDeletedVObject: []string{
				"sc",
			},
		},
		"pStorageClass exists, vStorageClass exists with different spec": {
			ExistingObjectInSuper: []runtime.Object{
				makeStorageClass("sc", "12345", func(class *v1.StorageClass) {
					class.Labels = map[string]string{
						constants.PublicObjectKey: "true",
					}
					class.Provisioner = "a"
				}),
			},
			ExistingObjectInTenant: []runtime.Object{
				makeStorageClass("sc", "123456", func(class *v1.StorageClass) {
					class.Provisioner = "b"
				}),
			},
			ExpectedUpdatedVObject: []runtime.Object{
				makeStorageClass("sc", "123456", func(class *v1.StorageClass) {
					class.Provisioner = "a"
				}),
			},
			WaitUWS: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tenantActions, superActions, err := util.RunPatrol(NewStorageClassController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, nil, tc.WaitDWS, tc.WaitUWS, nil)
			if err != nil {
				t.Errorf("%s: error running patrol: %v", k, err)
				return
			}

			if tc.ExpectedNoOperation {
				if len(superActions) != 0 {
					t.Errorf("%s: Expect no operation, got %v in super cluster", k, superActions)
					return
				}
				if len(tenantActions) != 0 {
					t.Errorf("%s: Expect no operation, got %v tenant cluster", k, tenantActions)
					return
				}
				return
			}

			for _, expectedName := range tc.ExpectedDeletedVObject {
				matched := false
				for _, action := range tenantActions {
					if !action.Matches("delete", "storageclasses") {
						continue
					}
					fullName := action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pStorageClass %s, got %s", k, expectedName, fullName)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect to delete pStorageClass %s, but not found", k, expectedName)
				}
			}

			for _, expectedName := range tc.ExpectedCreatedVObject {
				matched := false
				for _, action := range tenantActions {
					if !action.Matches("create", "storageclasses") {
						continue
					}
					created := action.(core.CreateAction).GetObject().(*v1.StorageClass)
					if created.Name != expectedName {
						t.Errorf("%s: Expect to create pStorageClass %s, got %s", k, expectedName, created.Name)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect to create pStorageClass %s, but not found", k, expectedName)
				}
			}

			for _, obj := range tc.ExpectedUpdatedVObject {
				matched := false
				for _, action := range tenantActions {
					if !action.Matches("update", "storageclasses") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated pStorageClass is %v, got %v", k, obj, actionObj)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect to update pStorageClass %s, but not found", k, obj)
				}
			}
		})
	}
}
