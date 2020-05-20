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

package service

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

func TestServicePatrol(t *testing.T) {
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

	spec1 := &v1.ServiceSpec{
		Type:      v1.ServiceTypeClusterIP,
		ClusterIP: "1.1.1.1",
		Selector: map[string]string{
			"a": "b",
		},
	}

	spec2 := &v1.ServiceSpec{
		Type:      v1.ServiceTypeClusterIP,
		ClusterIP: "3.3.3.3",
		Selector: map[string]string{
			"b": "c",
		},
	}

	spec3 := &v1.ServiceSpec{
		Type:      v1.ServiceTypeClusterIP,
		ClusterIP: "1.1.1.1",
		Selector: map[string]string{
			"b": "c",
		},
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedDeletedPObject []string
		ExpectedCreatedPObject []string
		ExpectedUpdatedPObject []runtime.Object
		ExpectedUpdatedVObject []runtime.Object
		ExpectedNoOperation    bool
		WaitDWS                bool // Make sure to set this flag if the test involves DWS.
		WaitUWS                bool // Make sure to set this flag if the test involves UWS.
	}{
		"pService not created by vc": {
			ExistingObjectInSuper: []runtime.Object{
				tenantService("svc-1", superDefaultNSName, "12345"),
			},
			ExpectedNoOperation: true,
		},
		"pService exists, vService does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc-2", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName + "/svc-2",
			},
		},
		"pService exists, vService exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc-3", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantService("svc-3", "default", "123456"),
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName + "/svc-3",
			},
		},
		"pService exists, vService exists with different spec": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToService(superService("svc-3", superDefaultNSName, "12345", defaultClusterKey), spec1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToService(tenantService("svc-3", "default", "12345"), spec2),
			},
			ExpectedUpdatedPObject: []runtime.Object{
				applySpecToService(superService("svc-3", superDefaultNSName, "12345", defaultClusterKey), spec3),
			},
			WaitDWS: true,
		},
		"pService exists, vService exists with different status": {
			ExistingObjectInSuper: []runtime.Object{
				applyLoadBalancerToService(superService("svc-3", superDefaultNSName, "12345", defaultClusterKey), "1.1.1.1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyLoadBalancerToService(tenantService("svc-3", "default", "12345"), "2.2.2.2"),
			},
			ExpectedUpdatedVObject: []runtime.Object{
				applyLoadBalancerToService(tenantService("svc-3", "default", "12345"), "1.1.1.1"),
			},
			WaitUWS: true,
		},
		"pService exists, vService exists with no diff": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc-3", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantService("svc-3", "default", "12345"),
			},
			ExpectedNoOperation: true,
		},
		"vService exists, pService does not exists": {
			ExistingObjectInTenant: []runtime.Object{
				tenantService("svc-5", "default", "12345"),
			},
			ExpectedCreatedPObject: []string{
				superDefaultNSName + "/svc-5",
			},
			WaitDWS: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tenantActions, superActions, err := util.RunPatrol(NewServiceController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.WaitDWS, tc.WaitUWS, nil)
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
					t.Errorf("%s: Expected to delete pService %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedPObject {
					action := superActions[i]
					if !action.Matches("delete", "services") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pService %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedCreatedPObject != nil {
				if len(tc.ExpectedCreatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to create PService %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedCreatedPObject {
					action := superActions[i]
					if !action.Matches("create", "services") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					created := action.(core.CreateAction).GetObject().(*v1.Service)
					fullName := created.Namespace + "/" + created.Name
					if fullName != expectedName {
						t.Errorf("%s: Expect to create pService %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedUpdatedPObject != nil {
				if len(tc.ExpectedUpdatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to update PService %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPObject, superActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedPObject {
					action := superActions[i]
					if !action.Matches("update", "services") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated pService is %v, got %v", k, obj, actionObj)
					}
				}
			}
			if tc.ExpectedUpdatedVObject != nil {
				if len(tc.ExpectedUpdatedVObject) != len(tenantActions) {
					t.Errorf("%s: Expected to update VPVC %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedVObject, tenantActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedVObject {
					action := tenantActions[i]
					if !action.Matches("update", "services") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated vPVC is %v, got %v", k, obj, actionObj)
					}
				}
			}
		})
	}
}
