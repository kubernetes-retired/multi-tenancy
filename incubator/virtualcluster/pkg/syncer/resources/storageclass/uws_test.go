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
	"encoding/json"
	"strings"
	"testing"

	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func makeStorageClass(name, uid string, mFuncs ...func(*v1.StorageClass)) *v1.StorageClass {
	sc := &v1.StorageClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "StorageClass",
			APIVersion: "storage.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
		Parameters: map[string]string{
			"type": "a",
		},
		Provisioner: "p1",
	}

	for _, f := range mFuncs {
		f(sc)
	}
	return sc
}

func TestUWPVCreation(t *testing.T) {
	testTenant := &v1alpha1.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "tenant-1",
			UID:       "7374a172-c35d-45b1-9c8e-bf5c5b614937",
		},
		Status: v1alpha1.VirtualClusterStatus{
			Phase: v1alpha1.ClusterRunning,
		},
	}

	defaultClusterKey := conversion.ToClusterKey(testTenant)

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		EnqueuedKey            string
		ExpectedCreatedObject  []string
		ExpectedError          string
		ExpectedNoOperation    bool
	}{
		"pSC exists but vSC not found": {
			ExistingObjectInSuper: []runtime.Object{
				makeStorageClass("sc", "12345"),
			},
			EnqueuedKey: defaultClusterKey + "/sc",
			ExpectedCreatedObject: []string{
				"sc",
			},
		},
		"pSC exists, vSC exists": {
			ExistingObjectInSuper: []runtime.Object{
				makeStorageClass("sc", "12345"),
			},
			ExistingObjectInTenant: []runtime.Object{
				makeStorageClass("sc", "123456"),
			},
			EnqueuedKey:         defaultClusterKey + "/sc",
			ExpectedNoOperation: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewStorageClassController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, nil)
			if err != nil {
				t.Errorf("%s: error running upward sync: %v", k, err)
				return
			}

			if tc.ExpectedNoOperation {
				if len(actions) != 0 {
					t.Errorf("%s: Expect no operation, got %v", k, actions)
					return
				}
				return
			}

			if reconcileErr != nil {
				if tc.ExpectedError == "" {
					t.Errorf("expected no error, but got \"%v\"", reconcileErr)
				} else if !strings.Contains(reconcileErr.Error(), tc.ExpectedError) {
					t.Errorf("expected error msg \"%s\", but got \"%v\"", tc.ExpectedError, reconcileErr)
				}
			} else {
				if tc.ExpectedError != "" {
					t.Errorf("expected error msg \"%s\", but got empty", tc.ExpectedError)
				}
			}

			for _, expectedName := range tc.ExpectedCreatedObject {
				matched := false
				for _, action := range actions {
					if !action.Matches("create", "storageclasses") {
						continue
					}
					created := action.(core.CreateAction).GetObject().(*v1.StorageClass)
					if created.Name != expectedName {
						t.Errorf("%s: Expected created vPV %s, got %s", k, expectedName, created.Name)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated pv %+v but not found", k, expectedName)
				}
			}
		})
	}
}

func TestUWPVUpdate(t *testing.T) {
	testTenant := &v1alpha1.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "tenant-1",
			UID:       "7374a172-c35d-45b1-9c8e-bf5c5b614937",
		},
		Status: v1alpha1.VirtualClusterStatus{
			Phase: v1alpha1.ClusterRunning,
		},
	}

	defaultClusterKey := conversion.ToClusterKey(testTenant)

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		EnqueuedKey            string
		ExpectedUpdatedObject  []runtime.Object
		ExpectedError          string
		ExpectedNoOperation    bool
	}{
		"pSC exists, vSC exists with different spec": {
			ExistingObjectInSuper: []runtime.Object{
				makeStorageClass("sc", "12345", func(class *v1.StorageClass) {
					class.Provisioner = "a"
				}),
			},
			ExistingObjectInTenant: []runtime.Object{
				makeStorageClass("sc", "123456", func(class *v1.StorageClass) {
					class.Provisioner = "b"
				}),
			},
			EnqueuedKey: defaultClusterKey + "/sc",
			ExpectedUpdatedObject: []runtime.Object{
				makeStorageClass("sc", "123456", func(class *v1.StorageClass) {
					class.Provisioner = "a"
				}),
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewStorageClassController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, nil)
			if err != nil {
				t.Errorf("%s: error running upward sync: %v", k, err)
				return
			}

			if tc.ExpectedNoOperation {
				if len(actions) != 0 {
					t.Errorf("%s: Expect no operation, got %v", k, actions)
					return
				}
				return
			}

			if reconcileErr != nil {
				if tc.ExpectedError == "" {
					t.Errorf("expected no error, but got \"%v\"", reconcileErr)
				} else if !strings.Contains(reconcileErr.Error(), tc.ExpectedError) {
					t.Errorf("expected error msg \"%s\", but got \"%v\"", tc.ExpectedError, reconcileErr)
				}
			} else {
				if tc.ExpectedError != "" {
					t.Errorf("expected error msg \"%s\", but got empty", tc.ExpectedError)
				}
			}

			for _, obj := range tc.ExpectedUpdatedObject {
				matched := false
				for _, action := range actions {
					if !action.Matches("update", "storageclasses") {
						continue
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						exp, _ := json.Marshal(obj)
						got, _ := json.Marshal(actionObj)
						t.Errorf("%s: Expected updated storageClass is %v, got %v", k, string(exp), string(got))
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated storageClass %+v but not found", k, obj)
				}
			}
		})
	}
}

func TestUWPVDeletion(t *testing.T) {
	testTenant := &v1alpha1.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "tenant-1",
			UID:       "7374a172-c35d-45b1-9c8e-bf5c5b614937",
		},
		Status: v1alpha1.VirtualClusterStatus{
			Phase: v1alpha1.ClusterRunning,
		},
	}

	defaultClusterKey := conversion.ToClusterKey(testTenant)

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		EnqueuedKey            string
		ExpectedDeletedObject  []string
		ExpectedError          string
		ExpectedNoOperation    bool
	}{
		"pSC not found, vSC exists": {
			ExistingObjectInTenant: []runtime.Object{
				makeStorageClass("sc", "12345"),
			},
			EnqueuedKey: defaultClusterKey + "/sc",
			ExpectedDeletedObject: []string{
				"sc",
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewStorageClassController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, nil)
			if err != nil {
				t.Errorf("%s: error running upward sync: %v", k, err)
				return
			}

			if tc.ExpectedNoOperation {
				if len(actions) != 0 {
					t.Errorf("%s: Expect no operation, got %v", k, actions)
					return
				}
				return
			}

			if reconcileErr != nil {
				if tc.ExpectedError == "" {
					t.Errorf("expected no error, but got \"%v\"", reconcileErr)
				} else if !strings.Contains(reconcileErr.Error(), tc.ExpectedError) {
					t.Errorf("expected error msg \"%s\", but got \"%v\"", tc.ExpectedError, reconcileErr)
				}
			} else {
				if tc.ExpectedError != "" {
					t.Errorf("expected error msg \"%s\", but got empty", tc.ExpectedError)
				}
			}

			for _, expectedName := range tc.ExpectedDeletedObject {
				matched := false
				for _, action := range actions {
					if !action.Matches("delete", "storageclasses") {
						continue
					}
					deleted := action.(core.DeleteAction).GetName()
					if deleted != expectedName {
						t.Errorf("%s: Expected created vPV %s, got %s", k, expectedName, deleted)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated pv %+v but not found", k, expectedName)
				}
			}
		})
	}
}
