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
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/testing"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func tenantService(name, namespace, uid string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func superService(name, namespace, uid string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				constants.LabelUID: uid,
			},
		},
	}
}

func TestDWServiceCreation(t *testing.T) {
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
		ExistingObjectInTenant *v1.Service

		ExpectedCreatedServices []string
		ExpectedError           string
	}{
		"new service": {
			ExistingObjectInSuper:   []runtime.Object{},
			ExistingObjectInTenant:  tenantService("svc-1", "default", "12345"),
			ExpectedCreatedServices: []string{superDefaultNSName + "/svc-1"},
		},
		"new service but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc-1", superDefaultNSName, "12345"),
			},
			ExistingObjectInTenant:  tenantService("svc-1", "default", "12345"),
			ExpectedCreatedServices: []string{},
			ExpectedError:           "",
		},
		"new serivce but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc-1", superDefaultNSName, "123456"),
			},
			ExistingObjectInTenant:  tenantService("svc-1", "default", "12345"),
			ExpectedCreatedServices: []string{},
			ExpectedError:           "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewServiceController,
				testTenant,
				tc.ExistingObjectInSuper,
				[]runtime.Object{tc.ExistingObjectInTenant},
				tc.ExistingObjectInTenant)
			if err != nil {
				t.Errorf("%s: error running downward sync: %v", k, err)
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

			if len(tc.ExpectedCreatedServices) != len(actions) {
				t.Errorf("%s: Expected to create service %#v. Actual actions were: %#v", k, tc.ExpectedCreatedServices, actions)
				return
			}
			for i, expectedName := range tc.ExpectedCreatedServices {
				action := actions[i]
				if !action.Matches("create", "services") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				createdSVC := action.(core.CreateAction).GetObject().(*v1.Service)
				fullName := createdSVC.Namespace + "/" + createdSVC.Name
				if fullName != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, fullName)
				}
			}
		})
	}
}

func TestDWServiceDeletion(t *testing.T) {
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
		ExistingObjectInSuper []runtime.Object
		EnqueueObject         *v1.Service

		ExpectedDeletedServices []string
		ExpectedError           string
	}{
		"delete service": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc-1", superDefaultNSName, "12345"),
			},
			EnqueueObject:           tenantService("svc-1", "default", "12345"),
			ExpectedDeletedServices: []string{superDefaultNSName + "/svc-1"},
		},
		"delete service but already gone": {
			ExistingObjectInSuper:   []runtime.Object{},
			EnqueueObject:           tenantService("svc-1", "default", "12345"),
			ExpectedDeletedServices: []string{},
			ExpectedError:           "",
		},
		"delete service but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc-1", superDefaultNSName, "123456"),
			},
			EnqueueObject:           tenantService("svc-1", "default", "12345"),
			ExpectedDeletedServices: []string{},
			ExpectedError:           "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewServiceController, testTenant, tc.ExistingObjectInSuper, nil, tc.EnqueueObject)
			if err != nil {
				t.Errorf("%s: error running downward sync: %v", k, err)
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

			if len(tc.ExpectedDeletedServices) != len(actions) {
				t.Errorf("%s: Expected to delete service %#v. Actual actions were: %#v", k, tc.ExpectedDeletedServices, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedServices {
				action := actions[i]
				if !action.Matches("delete", "services") {
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

func applySpecToService(svc *v1.Service, spec *v1.ServiceSpec) *v1.Service {
	svc.Spec = *spec.DeepCopy()
	return svc
}

func TestDWServiceUpdate(t *testing.T) {
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

	spec1 := &v1.ServiceSpec{
		Type:      "ClusterIP",
		ClusterIP: "1.1.1.1",
		Selector: map[string]string{
			"a": "b",
		},
	}

	spec2 := &v1.ServiceSpec{
		Type:      "ClusterIP",
		ClusterIP: "2.2.2.2",
		Selector: map[string]string{
			"a": "b",
		},
	}

	spec3 := &v1.ServiceSpec{
		Type:      "ClusterIP",
		ClusterIP: "3.3.3.3",
		Selector: map[string]string{
			"b": "c",
		},
	}

	spec4 := &v1.ServiceSpec{
		Type:      "ClusterIP",
		ClusterIP: "1.1.1.1",
		Selector: map[string]string{
			"b": "c",
		},
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant *v1.Service

		ExpectedUpdatedServices []runtime.Object
		ExpectedError           string
	}{
		"no diff": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToService(superService("svc-1", superDefaultNSName, "12345"), spec1),
			},
			ExistingObjectInTenant:  applySpecToService(tenantService("svc-1", "default", "12345"), spec2),
			ExpectedUpdatedServices: []runtime.Object{},
		},
		"diff in selector": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToService(superService("svc-1", superDefaultNSName, "12345"), spec1),
			},
			ExistingObjectInTenant: applySpecToService(tenantService("svc-1", "default", "12345"), spec3),
			ExpectedUpdatedServices: []runtime.Object{
				applySpecToService(superService("svc-1", superDefaultNSName, "12345"), spec4),
			},
		},
		"diff exists but uid is wrong": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToService(superService("svc-1", superDefaultNSName, "12345"), spec1),
			},
			ExistingObjectInTenant:  applySpecToService(tenantService("svc-1", "default", "123456"), spec3),
			ExpectedUpdatedServices: []runtime.Object{},
			ExpectedError:           "delegated UID is different",
		},
	}
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewServiceController,
				testTenant,
				tc.ExistingObjectInSuper,
				[]runtime.Object{tc.ExistingObjectInTenant},
				tc.ExistingObjectInTenant)
			if err != nil {
				t.Errorf("%s: error running downward sync: %v", k, err)
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

			if len(tc.ExpectedUpdatedServices) != len(actions) {
				t.Errorf("%s: Expected to update service %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedServices, actions)
				return
			}
			for i, obj := range tc.ExpectedUpdatedServices {
				action := actions[i]
				if !action.Matches("update", "services") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				actionObj := action.(core.UpdateAction).GetObject()
				if !equality.Semantic.DeepEqual(obj, actionObj) {
					t.Errorf("%s: Expected updated service is %v, got %v", k, obj, actionObj)
				}
			}
		})
	}
}
