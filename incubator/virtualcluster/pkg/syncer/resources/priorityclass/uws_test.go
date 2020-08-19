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

package priorityclass

import (
	"encoding/json"
	"strings"
	"testing"

	v1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func makePriorityClass(name, uid string, mFuncs ...func(*v1.PriorityClass)) *v1.PriorityClass {
	pc := &v1.PriorityClass{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PriorityClass",
			APIVersion: "priority.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
	}

	for _, f := range mFuncs {
		f(pc)
	}
	return pc
}

func TestUWPCCreation(t *testing.T) {
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
		"pPC exists but vPC not found": {
			ExistingObjectInSuper: []runtime.Object{
				makePriorityClass("pc", "12345"),
			},
			EnqueuedKey: defaultClusterKey + "/pc",
			ExpectedCreatedObject: []string{
				"pc",
			},
		},
		"pPC exists, vPC exists": {
			ExistingObjectInSuper: []runtime.Object{
				makePriorityClass("pc", "12345"),
			},
			ExistingObjectInTenant: []runtime.Object{
				makePriorityClass("pc", "123456"),
			},
			EnqueuedKey:         defaultClusterKey + "/pc",
			ExpectedNoOperation: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewPriorityClassController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, nil)
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
					if !action.Matches("create", "priorityclasses") {
						continue
					}
					created := action.(core.CreateAction).GetObject().(*v1.PriorityClass)
					if created.Name != expectedName {
						t.Errorf("%s: Expected created vPC %s, got %s", k, expectedName, created.Name)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated pc %+v but not found", k, expectedName)
				}
			}
		})
	}
}

func TestUWPCUpdate(t *testing.T) {
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
		"pPC exists, vPC exists with different spec": {
			ExistingObjectInSuper: []runtime.Object{
				makePriorityClass("pc", "12345", func(class *v1.PriorityClass) {
				}),
			},
			ExistingObjectInTenant: []runtime.Object{
				makePriorityClass("pc", "123456", func(class *v1.PriorityClass) {
				}),
			},
			EnqueuedKey: defaultClusterKey + "/pc",
			ExpectedUpdatedObject: []runtime.Object{
				makePriorityClass("pc", "123456", func(class *v1.PriorityClass) {
				}),
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewPriorityClassController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, nil)
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
					if !action.Matches("update", "priorityclasses") {
						continue
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						exp, _ := json.Marshal(obj)
						got, _ := json.Marshal(actionObj)
						t.Errorf("%s: Expected updated priorityClass is %v, got %v", k, string(exp), string(got))
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated priorityClass %+v but not found", k, obj)
				}
			}
		})
	}
}

func TestUWPCDeletion(t *testing.T) {
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
		"pPC not found, vPC exists": {
			ExistingObjectInTenant: []runtime.Object{
				makePriorityClass("pc", "12345"),
			},
			EnqueuedKey: defaultClusterKey + "/pc",
			ExpectedDeletedObject: []string{
				"pc",
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewPriorityClassController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, nil)
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
					if !action.Matches("delete", "priorityclasses") {
						continue
					}
					deleted := action.(core.DeleteAction).GetName()
					if deleted != expectedName {
						t.Errorf("%s: Expected created vPC %s, got %s", k, expectedName, deleted)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated pc %+v but not found", k, expectedName)
				}
			}
		})
	}
}
