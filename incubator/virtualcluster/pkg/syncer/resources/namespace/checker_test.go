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

package namespace

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

func superGCCandidate(name, uid, clusterKey, vcName, vcNamespace, vcUID, root string) *v1.Namespace {
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				constants.LabelUID:         uid,
				constants.LabelCluster:     clusterKey,
				constants.LabelNamespace:   "default",
				constants.LabelVCName:      vcName,
				constants.LabelVCNamespace: vcNamespace,
				constants.LabelVCUID:       vcUID,
				constants.LabelVCRootNS:    root,
			},
		},
	}
}

func TestNamespacePatrol(t *testing.T) {
	testTenant := &v1alpha1.VirtualCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualCluster",
			APIVersion: "tenancy/v1alpha1",
		},
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
		ExistingObjectInSuper    []runtime.Object
		ExistingObjectInTenant   []runtime.Object
		ExistingObjectInVCClient []runtime.Object
		ExpectedDeletedPObject   []string
		ExpectedCreatedPObject   []string
		ExpectedUpdatedPObject   []runtime.Object
		ExpectedNoOperation      bool
		WaitDWS                  bool // Make sure to set this flag if the test involves DWS.
		WaitUWS                  bool // Make sure to set this flag if the test involves UWS.
	}{
		"pNS not created by vc": {
			ExistingObjectInSuper: []runtime.Object{
				unknownNamespace(superDefaultNSName, "12345"),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedNoOperation: true,
		},
		"pNS exists, vNS does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superNamespace(superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName,
			},
		},
		"pNS exists, vNS exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superNamespace(superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantNamespace("default", "123456"),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName,
			},
		},
		"vNS exists, pNS does not exists": {
			ExistingObjectInTenant: []runtime.Object{
				tenantNamespace("default", "12345"),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedCreatedPObject: []string{
				superDefaultNSName,
			},
			WaitDWS: true,
		},
		"pNS's owner vc does not exist ": {
			ExistingObjectInSuper: []runtime.Object{
				superGCCandidate(superDefaultNSName, "12345", "12345", "test1", "default", "123456", "false"),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName,
			},
		},
		"rootns's owner vc does not exist ": {
			ExistingObjectInSuper: []runtime.Object{
				superGCCandidate(superDefaultNSName, "", defaultClusterKey, "test1", "default", "123456", "true"),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName,
			},
		},
		"rootns's owner vc exists ": {
			ExistingObjectInSuper: []runtime.Object{
				superGCCandidate(superDefaultNSName, "", defaultClusterKey, "test", "tenant-1", "7374a172-c35d-45b1-9c8e-bf5c5b614937", "true"),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedNoOperation: true,
		},
		"rootns's owner vc uid mismatch ": {
			ExistingObjectInSuper: []runtime.Object{
				superGCCandidate(superDefaultNSName, "", defaultClusterKey, "test", "tenant-1", "123456", "true"),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName,
			},
		},
		"pNS's owner vc exists but not managed by syncer ": {
			ExistingObjectInSuper: []runtime.Object{
				superGCCandidate(superDefaultNSName, "12345", "", "test", "tenant-1", "7374a172-c35d-45b1-9c8e-bf5c5b614937", "false"),
			},
			ExistingObjectInVCClient: []runtime.Object{
				testTenant,
			},
			ExpectedNoOperation: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tenantActions, superActions, err := util.RunPatrol(NewNamespaceController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInVCClient, tc.WaitDWS, tc.WaitUWS, nil)
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
					t.Errorf("%s: Expected to delete pPVC %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedPObject {
					action := superActions[i]
					if !action.Matches("delete", "namespaces") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pPVC %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedCreatedPObject != nil {
				if len(tc.ExpectedCreatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to create PPVC %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedCreatedPObject {
					action := superActions[i]
					if !action.Matches("create", "namespaces") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					created := action.(core.CreateAction).GetObject().(*v1.Namespace)
					fullName := created.Name
					if fullName != expectedName {
						t.Errorf("%s: Expect to create pPVC %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedUpdatedPObject != nil {
				if len(tc.ExpectedUpdatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to update PPVC %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPObject, superActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedPObject {
					action := superActions[i]
					if !action.Matches("update", "namespaces") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated pPVC is %v, got %v", k, obj, actionObj)
					}
				}
			}
		})
	}
}
