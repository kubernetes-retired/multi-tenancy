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

package secret

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

func applyGeneratedNameToSecret(secret *v1.Secret, name string) *v1.Secret {
	secret.Name = name
	return secret
}

func TestSecretPatrol(t *testing.T) {
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
		ExpectedDeletedPObject []string
		ExpectedCreatedPObject []runtime.Object
		ExpectedUpdatedPObject []runtime.Object
		ExpectedNoOperation    bool
		WaitDWS                bool // Make sure to set this flag if the test involves DWS.
		WaitUWS                bool // Make sure to set this flag if the test involves UWS.
	}{
		"pSecret not created by vc": {
			ExistingObjectInSuper: []runtime.Object{
				tenantSecret("secret", superDefaultNSName, "12345", v1.SecretTypeOpaque),
			},
			ExpectedNoOperation: true,
		},
		"pSecret with service account type created by token controller": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeServiceAccountToken),
			},
			ExpectedNoOperation: true,
		},
		"pSecret exists, vSecret does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque),
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName + "/normal-secret",
			},
		},
		"pSecret exists, vSecret exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("normal-secret", "default", "123456", v1.SecretTypeOpaque),
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName + "/normal-secret",
			},
		},
		"pSecret exists, vSecret exists with no diff": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque),
			},
			ExpectedNoOperation: true,
		},
		"pSecret exists, vSecret exists but different in data": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToSecret(superSecret("normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque), "data1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToSecret(tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque), "data2"),
			},
			ExpectedNoOperation: true,
		},
		"vSecret exists, pSecret does not exists": {
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque),
			},
			ExpectedCreatedPObject: []runtime.Object{
				superSecret("normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque),
			},
			WaitDWS: true,
		},
		"pSecret exists, vSecret does not exists, service account token type": {
			ExistingObjectInSuper: []runtime.Object{
				applyGeneratedNameToSecret(superServiceAccountSecret("sa-secret", superDefaultNSName, "12345", defaultClusterKey), "sa-secret-token-xxx"),
			},
			ExpectedDeletedPObject: []string{
				superDefaultNSName + "/sa-secret-token-xxx",
			},
		},
		"vSecret exists, pSecret does not exists, service account token type": {
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken),
			},
			ExpectedCreatedPObject: []runtime.Object{
				superServiceAccountSecret("sa-secret", superDefaultNSName, "12345", defaultClusterKey),
			},
			WaitDWS: true,
		},
		"vSecret exists, pSecret exists with different data, service account token type": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToSecret(superServiceAccountSecret("sa-secret", superDefaultNSName, "12345", defaultClusterKey), "data1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToSecret(tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken), "data2"),
			},
			ExpectedNoOperation: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tenantActions, superActions, err := util.RunPatrol(NewSecretController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.WaitDWS, tc.WaitUWS, nil)
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
					t.Errorf("%s: Expected to delete pObject %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedPObject {
					action := superActions[i]
					if !action.Matches("delete", "secrets") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pObject %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedCreatedPObject != nil {
				if len(tc.ExpectedCreatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to create PObject %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPObject, superActions)
					return
				}
				for i, expectedObject := range tc.ExpectedCreatedPObject {
					action := superActions[i]
					if !action.Matches("create", "secrets") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					got := action.(core.CreateAction).GetObject().(*v1.Secret)
					expectedSecret := expectedObject.(*v1.Secret)
					if !equality.Semantic.DeepEqual(got, expectedSecret) {
						t.Errorf("%s: Expected secret %v, got %v: %v", k, expectedSecret, got, err)
					}
				}
			}
			if tc.ExpectedUpdatedPObject != nil {
				if len(tc.ExpectedUpdatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to update PObject %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPObject, superActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedPObject {
					action := superActions[i]
					if !action.Matches("update", "secrets") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated pObject is %v, got %v", k, obj, actionObj)
					}
				}
			}
		})
	}
}
