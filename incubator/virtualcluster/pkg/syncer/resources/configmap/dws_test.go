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

package configmap

import (
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/testing"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

func tenantConfigMap(name, namespace, uid string) *v1.ConfigMap {
	return &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "v1",
			APIVersion: "configmaps",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func superConfigMap(name, namespace, uid, clusterKey string) *v1.ConfigMap {
	return &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "v1",
			APIVersion: "configmaps",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				constants.LabelUID:       uid,
				constants.LabelCluster:   clusterKey,
				constants.LabelNamespace: "default",
			},
		},
	}
}

func TestDWConfigMapCreation(t *testing.T) {
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

	defaultClusterKey := conversion.ToClusterKey(testTenant)
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedCreatedPObject []string
		ExpectedNoOperation    bool
		ExpectedError          string
	}{
		"new cm": {
			ExistingObjectInSuper: []runtime.Object{},
			ExistingObjectInTenant: []runtime.Object{
				tenantConfigMap("cm-1", "default", "12345"),
			},
			ExpectedCreatedPObject: []string{superDefaultNSName + "/cm-1"},
		},
		"new cm but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superConfigMap("cm-2", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantConfigMap("cm-2", "default", "12345"),
			},
			ExpectedNoOperation: true,
		},
		"new cm but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superConfigMap("cm-3", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantConfigMap("cm-3", "default", "12345"),
			},
			ExpectedError: "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewConfigMapController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], nil)
			if err != nil {
				t.Errorf("%s: error running downward sync: %v", k, err)
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

			if len(tc.ExpectedCreatedPObject) != len(actions) {
				t.Errorf("%s: Expected to create cm %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPObject, actions)
				return
			}
			for i, expectedName := range tc.ExpectedCreatedPObject {
				action := actions[i]
				if !action.Matches("create", "configmaps") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				created := action.(core.CreateAction).GetObject().(*v1.ConfigMap)
				fullName := created.Namespace + "/" + created.Name
				if fullName != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, fullName)
				}
			}
		})
	}
}

func TestDWConfigMapDeletion(t *testing.T) {
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

	defaultClusterKey := conversion.ToClusterKey(testTenant)
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		EnqueueObject          *v1.ConfigMap
		ExpectedDeletedPObject []string
		ExpectedNoOperation    bool
		ExpectedError          string
	}{
		"delete cm": {
			ExistingObjectInSuper: []runtime.Object{
				superConfigMap("cm-1", superDefaultNSName, "12345", defaultClusterKey),
			},
			EnqueueObject:          tenantConfigMap("cm-1", "default", "12345"),
			ExpectedDeletedPObject: []string{superDefaultNSName + "/cm-1"},
		},
		"delete cm but already gone": {
			ExistingObjectInSuper: []runtime.Object{},
			EnqueueObject:         tenantConfigMap("cm-2", "default", "12345"),
			ExpectedNoOperation:   true,
		},
		"delete cm but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superConfigMap("cm-3", superDefaultNSName, "123456", defaultClusterKey),
			},
			EnqueueObject: tenantConfigMap("cm-3", "default", "12345"),
			ExpectedError: "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewConfigMapController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueueObject, nil)
			if err != nil {
				t.Errorf("%s: error running downward sync: %v", k, err)
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

			if len(tc.ExpectedDeletedPObject) != len(actions) {
				t.Errorf("%s: Expected to delete cm %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPObject, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedPObject {
				action := actions[i]
				if !action.Matches("delete", "configmaps") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
				if fullName != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, fullName)
				}
			}
		})
	}
}

func applyDataToConfigMap(cm *v1.ConfigMap, data string) *v1.ConfigMap {
	cm.Data = map[string]string{
		data: data,
	}
	return cm
}

func TestDWConfigMapUpdate(t *testing.T) {
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

	defaultClusterKey := conversion.ToClusterKey(testTenant)
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	data1 := "data1"
	data2 := "data2"

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedUpdatedPObject []runtime.Object
		ExpectedNoOperation    bool
		ExpectedError          string
	}{
		"no diff": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToConfigMap(superConfigMap("cm-1", superDefaultNSName, "12345", defaultClusterKey), data1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToConfigMap(tenantConfigMap("cm-1", "default", "12345"), data1),
			},
			ExpectedNoOperation: true,
		},
		"diff in data": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToConfigMap(superConfigMap("cm-2", superDefaultNSName, "12345", defaultClusterKey), data1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToConfigMap(tenantConfigMap("cm-2", "default", "12345"), data2),
			},
			ExpectedUpdatedPObject: []runtime.Object{
				applyDataToConfigMap(superConfigMap("cm-2", superDefaultNSName, "12345", defaultClusterKey), data2),
			},
		},
		"diff exists but uid is wrong": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToConfigMap(superConfigMap("cm-3", superDefaultNSName, "12345", defaultClusterKey), data1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToConfigMap(tenantConfigMap("cm-3", "default", "123456"), data2),
			},
			ExpectedError:       "delegated UID is different",
			ExpectedNoOperation: true,
		},
	}
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewConfigMapController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], nil)
			if err != nil {
				t.Errorf("%s: error running downward sync: %v", k, err)
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

			if len(tc.ExpectedUpdatedPObject) != len(actions) {
				t.Errorf("%s: Expected to update cm %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPObject, actions)
				return
			}
			for i, obj := range tc.ExpectedUpdatedPObject {
				action := actions[i]
				if !action.Matches("update", "configmaps") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				actionObj := action.(core.UpdateAction).GetObject()
				if !equality.Semantic.DeepEqual(obj, actionObj) {
					t.Errorf("%s: Expected updated cm is %v, got %v", k, obj, actionObj)
				}
			}
		})
	}
}
