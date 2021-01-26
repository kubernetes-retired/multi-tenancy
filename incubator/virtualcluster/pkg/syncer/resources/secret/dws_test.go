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
	"fmt"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

func superSecret(vcName, vcNamespace, name, namespace, uid, clusterKey string, secretType v1.SecretType) *v1.Secret {
	secret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelVCName:      vcName,
				constants.LabelVCNamespace: vcNamespace,
			},
			Annotations: map[string]string{
				constants.LabelUID:             uid,
				constants.LabelCluster:         clusterKey,
				constants.LabelNamespace:       "default",
				constants.LabelOwnerReferences: "null",
				constants.LabelVCName:          vcName,
				constants.LabelVCNamespace:     vcNamespace,
			},
		},
		Type: secretType,
	}

	return secret
}

func superServiceAccountSecret(vcName, vcNamespace, name, namespace, uid, clusterKey string) *v1.Secret {
	return &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:         "",
			Namespace:    namespace,
			GenerateName: "-token-",
			Annotations: map[string]string{
				constants.LabelUID:             uid,
				constants.LabelCluster:         clusterKey,
				constants.LabelNamespace:       "default",
				constants.LabelOwnerReferences: "null",
				constants.LabelSecretName:      name,
				constants.LabelVCName:          vcName,
				constants.LabelVCNamespace:     vcNamespace,
			},
			Labels: map[string]string{
				constants.LabelSecretUID:   uid,
				constants.LabelVCName:      vcName,
				constants.LabelVCNamespace: vcNamespace,
			},
		},
		Type: v1.SecretTypeOpaque,
	}
}

func tenantSecret(name, namespace, uid string, secretType v1.SecretType) *v1.Secret {
	return &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Type: secretType,
	}
}

func TestDWSecretCreation(t *testing.T) {
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
	defaultVCName, defaultVCNamespace := testTenant.Name, testTenant.Namespace
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedCreatedPObject []runtime.Object
		ExpectedError          string
		ExpectedNoOperation    bool
	}{
		"new secret": {
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque),
			},
			ExpectedCreatedPObject: []runtime.Object{
				superSecret(defaultVCName, defaultVCNamespace, "normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque),
			},
		},
		"new secret but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret(defaultVCName, defaultVCNamespace, "normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque),
			},
			ExpectedNoOperation: true,
		},
		"new secret but exists different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret(defaultVCName, defaultVCNamespace, "normal-secret", superDefaultNSName, "123456", defaultClusterKey, v1.SecretTypeOpaque),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque),
			},
			ExpectedError: "delegated UID is different",
		},
		"new secret but conflict with generated sa opaque secret": {
			ExistingObjectInSuper: []runtime.Object{
				applyGeneratedNameToSecret(superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "123456", defaultClusterKey), "normal-token-1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("normal-token-1", "default", "12345", v1.SecretTypeOpaque),
			},
			ExpectedError: "delegated UID is different",
		},
		"new service account secret": {
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken),
			},
			ExpectedCreatedPObject: []runtime.Object{
				superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey),
			},
		},
		"new service account secret when token controller created one exists": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeServiceAccountToken),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken),
			},
			ExpectedCreatedPObject: []runtime.Object{
				superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey),
			},
		},
		"new service account secret but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken),
			},
			ExpectedNoOperation: true,
		},
		"new service account secret but exists different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken),
			},
			ExpectedCreatedPObject: []runtime.Object{
				superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey),
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewSecretController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], func(tenantClientset, superClientset *fake.Clientset) {
				superClientset.PrependReactor("create", "secrets", generateNameReactor)
			})
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
				t.Errorf("%s: Expected to create Secret %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPObject, actions)
				return
			}
			for i, expectedObject := range tc.ExpectedCreatedPObject {
				action := actions[i]
				if !action.Matches("create", "secrets") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				got := action.(core.CreateAction).GetObject().(*v1.Secret)
				expectedSecret := expectedObject.(*v1.Secret)
				if !equality.Semantic.DeepEqual(got, expectedSecret) {
					t.Errorf("%s: Expected secret %v, got %v: %v", k, expectedSecret, got, err)
				}
			}
		})
	}
}

func TestDWSecretDeletion(t *testing.T) {
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
	defaultVCName, defaultVCNamespace := testTenant.Name, testTenant.Namespace
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedDeletedPObject []string
		EnqueueObject          *v1.Secret
		ExpectedError          string
		ExpectedNoOperation    bool
	}{
		"delete secret": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret(defaultVCName, defaultVCNamespace, "normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque),
			},
			EnqueueObject:          tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque),
			ExpectedDeletedPObject: []string{superDefaultNSName + "/normal-secret"},
		},
		"delete secret but already gone": {
			EnqueueObject:       tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque),
			ExpectedNoOperation: true,
		},
		"delete secret but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret(defaultVCName, defaultVCNamespace, "normal-secret", superDefaultNSName, "123456", defaultClusterKey, v1.SecretTypeOpaque),
			},
			EnqueueObject: tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque),
			ExpectedError: "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewSecretController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueueObject, func(tenantClientset, superClientset *fake.Clientset) {
				superClientset.PrependReactor("create", "secrets", generateNameReactor)
			})
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
				t.Errorf("%s: Expected to delete Secret %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPObject, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedPObject {
				action := actions[i]
				if !action.Matches("delete", "secrets") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
				if fullName != expectedName {
					t.Errorf("%s: Expected %s to be deleted, got %s", k, expectedName, fullName)
				}
			}
		})
	}
}

func TestDWServiceAccountSecretDeletion(t *testing.T) {
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
	defaultVCName, defaultVCNamespace := testTenant.Name, testTenant.Namespace
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper     []runtime.Object
		ExistingObjectInTenant    []runtime.Object
		ExpectedDeletedCollection []string
		EnqueueObject             *v1.Secret
		ExpectedError             string
		ExpectedNoOperation       bool
	}{
		"delete service account secret": {
			ExistingObjectInSuper: []runtime.Object{
				superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey),
			},
			EnqueueObject:             tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken),
			ExpectedDeletedCollection: []string{constants.LabelSecretUID + "=12345"},
		},
		"delete service account secret but already gone": {
			EnqueueObject:       tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken),
			ExpectedNoOperation: true,
		},
		"delete service account secret but existing different one": {
			ExistingObjectInSuper: []runtime.Object{
				superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "123456", defaultClusterKey),
			},
			EnqueueObject:       tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken),
			ExpectedNoOperation: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewSecretController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueueObject, func(tenantClientset, superClientset *fake.Clientset) {
				superClientset.PrependReactor("create", "secrets", generateNameReactor)
			})
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

			if len(tc.ExpectedDeletedCollection) != len(actions) {
				t.Errorf("%s: Expected to delete Secret %#v. Actual actions were: %#v", k, tc.ExpectedDeletedCollection, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedCollection {
				action := actions[i]
				if !action.Matches("delete-collection", "secrets") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				got := action.(core.DeleteCollectionAction).GetListRestrictions().Labels.String()
				if got != expectedName {
					t.Errorf("%s: Expected %s to be deleted, got %s", k, expectedName, got)
				}
			}
		})
	}
}

func applyDataToSecret(secret *v1.Secret, data string) *v1.Secret {
	secret.Data = map[string][]byte{
		data: []byte(data),
	}
	return secret
}

func TestDWSecretUpdate(t *testing.T) {
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
	defaultVCName, defaultVCNamespace := testTenant.Name, testTenant.Namespace
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedUpdatedPObject []runtime.Object
		ExpectedError          string
		ExpectedNoOperation    bool
	}{
		"secret no diff": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToSecret(superSecret(defaultVCName, defaultVCNamespace, "normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque), "data1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToSecret(tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque), "data1"),
			},
			ExpectedNoOperation: true,
		},
		"secret diff in data": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToSecret(superSecret(defaultVCName, defaultVCNamespace, "normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque), "data1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToSecret(tenantSecret("normal-secret", "default", "12345", v1.SecretTypeOpaque), "data2"),
			},
			ExpectedUpdatedPObject: []runtime.Object{
				applyDataToSecret(superSecret(defaultVCName, defaultVCNamespace, "normal-secret", superDefaultNSName, "12345", defaultClusterKey, v1.SecretTypeOpaque), "data2"),
			},
		},
		"service account secret no diff": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToSecret(superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey), "data1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToSecret(tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken), "data1"),
			},
			ExpectedNoOperation: true,
		},
		"service account secret diff in data": {
			ExistingObjectInSuper: []runtime.Object{
				applyDataToSecret(superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey), "data1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDataToSecret(tenantSecret("sa-secret", "default", "12345", v1.SecretTypeServiceAccountToken), "data2"),
			},
			ExpectedUpdatedPObject: []runtime.Object{
				applyDataToSecret(superServiceAccountSecret(defaultVCName, defaultVCNamespace, "sa-secret", superDefaultNSName, "12345", defaultClusterKey), "data2"),
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewSecretController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], func(tenantClientset, superClientset *fake.Clientset) {
				superClientset.PrependReactor("create", "secrets", generateNameReactor)
			})
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
				t.Errorf("%s: Expected to create Secret %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPObject, actions)
				return
			}
			for i, expectedObject := range tc.ExpectedUpdatedPObject {
				action := actions[i]
				if !action.Matches("update", "secrets") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				got := action.(core.UpdateAction).GetObject().(*v1.Secret)
				expectedSecret := expectedObject.(*v1.Secret)
				if !equality.Semantic.DeepEqual(got, expectedSecret) {
					t.Errorf("%s: Expected secret %v, got %v: %v", k, expectedSecret, got, err)
				}
			}
		})
	}
}

// generateNameReactor implements the logic required for the GenerateName field to work when using
// the fake client. Add it with client.PrependReactor to your fake client.
func generateNameReactor(action core.Action) (handled bool, ret runtime.Object, err error) {
	s := action.(core.CreateAction).GetObject().(*v1.Secret)
	if s.Name == "" && s.GenerateName != "" {
		s.Name = fmt.Sprintf("%s-%s", s.GenerateName, rand.String(16))
	}
	return false, nil, nil
}
