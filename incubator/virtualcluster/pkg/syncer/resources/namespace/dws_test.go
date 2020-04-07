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
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/testing"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func tenantNamespace(name, uid string) *v1.Namespace {
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
	}
}

func superNamespace(name, uid string) *v1.Namespace {
	return &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				constants.LabelUID: uid,
			},
		},
	}
}

func TestDWNamespaceCreation(t *testing.T) {
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

	defaultNSName := "default"
	defaultClusterKey := conversion.ToClusterKey(testTenant)
	defaultSuperNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, defaultNSName)

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant *v1.Namespace

		ExpectedCreatedNamespace []string
		ExpectedError            string
	}{
		"new namespace": {
			ExistingObjectInSuper:    []runtime.Object{},
			ExistingObjectInTenant:   tenantNamespace(defaultNSName, "12345"),
			ExpectedCreatedNamespace: []string{defaultSuperNSName},
		},
		"new namespace but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superNamespace(defaultSuperNSName, "12345"),
			},
			ExistingObjectInTenant:   tenantNamespace(defaultNSName, "12345"),
			ExpectedCreatedNamespace: []string{},
			ExpectedError:            "",
		},
		"new namespace but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superNamespace(defaultSuperNSName, "123456"),
			},
			ExistingObjectInTenant:   tenantNamespace(defaultNSName, "12345"),
			ExpectedCreatedNamespace: []string{},
			ExpectedError:            "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewNamespaceController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant)
			if err != nil {
				t.Errorf("%s: error running downward sync: %v", k, err)
				return
			}

			if reconcileErr != nil {
				if tc.ExpectedError == "" {
					t.Errorf("expected no error, but got \"%v\"", err)
				} else if !strings.Contains(reconcileErr.Error(), tc.ExpectedError) {
					t.Errorf("expected error msg \"%s\", but got \"%v\"", tc.ExpectedError, err)
				}
			} else {
				if tc.ExpectedError != "" {
					t.Errorf("expected error msg \"%s\", but got empty", tc.ExpectedError)
				}
			}

			if len(tc.ExpectedCreatedNamespace) != len(actions) {
				t.Errorf("%s: Expected to create namespace %#v. Actual actions were: %#v", k, tc.ExpectedCreatedNamespace, actions)
				return
			}
			for i, expectedName := range tc.ExpectedCreatedNamespace {
				action := actions[i]
				if !action.Matches("create", "namespaces") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				createdNS := action.(core.CreateAction).GetObject().(*v1.Namespace)
				if createdNS.Name != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, createdNS.Name)
				}
			}
		})
	}
}

func TestDWNamespaceDeletion(t *testing.T) {
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

	defaultNSName := "default"
	defaultClusterKey := conversion.ToClusterKey(testTenant)
	defaultSuperNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, defaultNSName)

	testcases := map[string]struct {
		ExistingObjectInSuper []runtime.Object
		EnqueueObject         *v1.Namespace

		ExpectedDeletedNamespace []string
		ExpectedError            string
	}{
		"delete namespace": {
			ExistingObjectInSuper: []runtime.Object{
				superNamespace(defaultSuperNSName, "12345"),
			},
			EnqueueObject:            tenantNamespace(defaultNSName, "12345"),
			ExpectedDeletedNamespace: []string{defaultSuperNSName},
		},
		"delete namespace but already gone": {
			ExistingObjectInSuper:    []runtime.Object{},
			EnqueueObject:            tenantNamespace(defaultNSName, "12345"),
			ExpectedDeletedNamespace: []string{},
			ExpectedError:            "",
		},
		"delete namespace but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superNamespace(defaultSuperNSName, "123456"),
			},
			EnqueueObject:            tenantNamespace(defaultNSName, "12345"),
			ExpectedDeletedNamespace: []string{},
			ExpectedError:            "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewNamespaceController, testTenant, tc.ExistingObjectInSuper, nil, tc.EnqueueObject)
			if err != nil {
				t.Errorf("%s: error running downward sync: %v", k, err)
				return
			}

			if reconcileErr != nil {
				if tc.ExpectedError == "" {
					t.Errorf("expected no error, but got \"%v\"", err)
				} else if !strings.Contains(reconcileErr.Error(), tc.ExpectedError) {
					t.Errorf("expected error msg \"%s\", but got \"%v\"", tc.ExpectedError, err)
				}
			} else {
				if tc.ExpectedError != "" {
					t.Errorf("expected error msg \"%s\", but got empty", tc.ExpectedError)
				}
			}

			if len(tc.ExpectedDeletedNamespace) != len(actions) {
				t.Errorf("%s: Expected to delete namespace %#v. Actual actions were: %#v", k, tc.ExpectedDeletedNamespace, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedNamespace {
				action := actions[i]
				if !action.Matches("delete", "namespaces") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				deleteNS := action.(core.DeleteAction).GetName()
				if deleteNS != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, deleteNS)
				}
			}
		})
	}
}
