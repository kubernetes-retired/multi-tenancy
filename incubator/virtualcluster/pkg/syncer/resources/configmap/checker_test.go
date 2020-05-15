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
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

func TestConfigMapPatrol(t *testing.T) {
	testTenant := &v1alpha1.Virtualcluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "tenant-1",
			UID:       "7374a172-c35d-45b1-9c8e-bf5c5b614937",
		},
		Spec: v1alpha1.VirtualclusterSpec{},
		Status: v1alpha1.VirtualclusterStatus{
			Phase: v1alpha1.ClusterRunning,
		},
	}

	defaultClusterKey := conversion.ToClusterKey(testTenant)
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedDeletedPObject []string
		ExpectedDeletedVObject []string
		ExpectedCreatedPObject []string
		ExpectedUpdatedPObject []runtime.Object
		ExpectedUpdatedVObject []runtime.Object
		ExpectedNoOperation    bool
		WaitDWS                bool // Make sure to set this flag if the test involves DWS.
		WaitUWS                bool // Make sure to set this flag if the test involves UWS.
	}{
		"pConfigMap not created by vc": {
			ExistingObjectInSuper: []runtime.Object{
				tenantConfigMap("cm-1", superDefaultNSName, "12345"),
			},
			ExpectedNoOperation: true,
		},
		"pConfigMap exists, vConfigMap does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superConfigMap("cm-2", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName + "/cm-2",
			},
		},
		"pConfigMap exists, vConfigMap exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superConfigMap("cm-3", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantConfigMap("cm-3", "default", "123456"),
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName + "/cm-3",
			},
		},
		"pConfigMap exists, vConfigMap exists with different spec": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToConfigMap(superConfigMap("cm-4", superDefaultNSName, "12345", defaultClusterKey), "data1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToConfigMap(tenantConfigMap("cm-4", "default", "12345"), "data2"),
			},
			ExpectedNoOperation: true,
			// notes: have not updated the different pConfigMap in patrol now.
		},
		"vConfigMap exists, pConfigMap does not exists": {
			ExistingObjectInTenant: []runtime.Object{
				tenantConfigMap("cm-5", "default", "12345"),
			},
			ExpectedCreatedPObject: []string{
				superDefaultNSName + "/cm-5",
			},
			WaitDWS: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tenantActions, superActions, err := util.RunPatrol(NewConfigMapController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.WaitDWS, tc.WaitUWS, nil)
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

			if tc.ExpectedDeletedPObject != nil {
				if len(tc.ExpectedDeletedPObject) != len(superActions) {
					t.Errorf("%s: Expected to delete pConfigMap %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedPObject {
					action := superActions[i]
					if !action.Matches("delete", "configmaps") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pConfigMap %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedDeletedVObject != nil {
				if len(tc.ExpectedDeletedVObject) != len(tenantActions) {
					t.Errorf("%s: Expected to delete VConfigMap %#v. Actual actions were: %#v", k, tc.ExpectedDeletedVObject, tenantActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedVObject {
					action := tenantActions[i]
					if !action.Matches("delete", "configmaps") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pConfigMap %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedCreatedPObject != nil {
				if len(tc.ExpectedCreatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to create PConfigMap %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedCreatedPObject {
					action := superActions[i]
					if !action.Matches("create", "configmaps") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					created := action.(core.CreateAction).GetObject().(*v1.ConfigMap)
					fullName := created.Namespace + "/" + created.Name
					if fullName != expectedName {
						t.Errorf("%s: Expect to create pConfigMap %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedUpdatedPObject != nil {
				if len(tc.ExpectedUpdatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to update PConfigMap %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPObject, superActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedPObject {
					action := superActions[i]
					if !action.Matches("update", "configmaps") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated pConfigMap is %v, got %v", k, obj, actionObj)
					}
				}
			}
			if tc.ExpectedUpdatedVObject != nil {
				if len(tc.ExpectedUpdatedVObject) != len(tenantActions) {
					t.Errorf("%s: Expected to update VConfigMap %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedVObject, tenantActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedVObject {
					action := tenantActions[i]
					if !action.Matches("update", "configmaps") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated vConfigMap is %v, got %v", k, obj, actionObj)
					}
				}
			}
		})
	}
}
