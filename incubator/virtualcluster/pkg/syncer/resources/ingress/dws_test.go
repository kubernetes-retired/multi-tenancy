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

package ingress

import (
	"strings"
	"testing"

	v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/testing"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	utilscheme "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/scheme"
)

func tenantIngress(name, namespace, uid string) *v1beta1.Ingress {
	return &v1beta1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: "v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func superIngress(name, namespace, uid, clusterKey string) *v1beta1.Ingress {
	return &v1beta1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				constants.LabelUID:       uid,
				constants.LabelNamespace: "default",
				constants.LabelCluster:   clusterKey,
			},
		},
	}
}

func TestDWIngressCreation(t *testing.T) {
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
		ExistingObjectInTenant *v1beta1.Ingress

		ExpectedCreatedIngresses []string
		ExpectedError            string
	}{
		"new ingress": {
			ExistingObjectInSuper:    []runtime.Object{},
			ExistingObjectInTenant:   tenantIngress("ing-1", "default", "12345"),
			ExpectedCreatedIngresses: []string{superDefaultNSName + "/ing-1"},
		},
		"new ingress but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superIngress("ing-1", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant:   tenantIngress("ing-1", "default", "12345"),
			ExpectedCreatedIngresses: []string{},
			ExpectedError:            "",
		},
		"new serivce but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superIngress("ing-1", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant:   tenantIngress("ing-1", "default", "12345"),
			ExpectedCreatedIngresses: []string{},
			ExpectedError:            "delegated UID is different",
		},
	}

	utilscheme.Scheme.AddKnownTypePair(&v1beta1.Ingress{}, &v1beta1.IngressList{})
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewIngressController,
				testTenant,
				tc.ExistingObjectInSuper,
				[]runtime.Object{tc.ExistingObjectInTenant},
				tc.ExistingObjectInTenant,
				nil)
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

			if len(tc.ExpectedCreatedIngresses) != len(actions) {
				t.Errorf("%s: Expected to create ingress %#v. Actual actions were: %#v", k, tc.ExpectedCreatedIngresses, actions)
				return
			}
			for i, expectedName := range tc.ExpectedCreatedIngresses {
				action := actions[i]
				if !action.Matches("create", "ingresses") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				createdSVC := action.(core.CreateAction).GetObject().(*v1beta1.Ingress)
				fullName := createdSVC.Namespace + "/" + createdSVC.Name
				if fullName != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, fullName)
				}
			}
		})
	}
}

func TestDWIngressDeletion(t *testing.T) {
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
		ExistingObjectInSuper []runtime.Object
		EnqueueObject         *v1beta1.Ingress

		ExpectedDeletedIngresses []string
		ExpectedError            string
	}{
		"delete ingress": {
			ExistingObjectInSuper: []runtime.Object{
				superIngress("ing-1", superDefaultNSName, "12345", defaultClusterKey),
			},
			EnqueueObject:            tenantIngress("ing-1", "default", "12345"),
			ExpectedDeletedIngresses: []string{superDefaultNSName + "/ing-1"},
		},
		"delete ingress but already gone": {
			ExistingObjectInSuper:    []runtime.Object{},
			EnqueueObject:            tenantIngress("ing-1", "default", "12345"),
			ExpectedDeletedIngresses: []string{},
			ExpectedError:            "",
		},
		"delete ingress but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superIngress("ing-1", superDefaultNSName, "123456", defaultClusterKey),
			},
			EnqueueObject:            tenantIngress("ing-1", "default", "12345"),
			ExpectedDeletedIngresses: []string{},
			ExpectedError:            "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewIngressController, testTenant, tc.ExistingObjectInSuper, nil, tc.EnqueueObject, nil)
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

			if len(tc.ExpectedDeletedIngresses) != len(actions) {
				t.Errorf("%s: Expected to delete ingress %#v. Actual actions were: %#v", k, tc.ExpectedDeletedIngresses, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedIngresses {
				action := actions[i]
				if !action.Matches("delete", "ingresses") {
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

func applySpecToIngress(ing *v1beta1.Ingress, spec *v1beta1.IngressSpec) *v1beta1.Ingress {
	ing.Spec = *spec.DeepCopy()
	return ing
}

func TestDWIngressUpdate(t *testing.T) {
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

	nginx := "nginx"
	spec1 := &v1beta1.IngressSpec{
		IngressClassName: &nginx,
		Backend: &v1beta1.IngressBackend{
			ServiceName: "nginx",
		},
	}

	spec2 := &v1beta1.IngressSpec{
		IngressClassName: &nginx,
		Backend: &v1beta1.IngressBackend{
			ServiceName: "nginx",
		},
	}

	haproxy := "haproxy"
	spec3 := &v1beta1.IngressSpec{
		IngressClassName: &haproxy,
		Backend: &v1beta1.IngressBackend{
			ServiceName: "haproxy",
		},
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant *v1beta1.Ingress

		ExpectedUpdatedIngresses []runtime.Object
		ExpectedError            string
	}{
		"no diff": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToIngress(superIngress("ing-1", superDefaultNSName, "12345", defaultClusterKey), spec1),
			},
			ExistingObjectInTenant:   applySpecToIngress(tenantIngress("ing-1", "default", "12345"), spec2),
			ExpectedUpdatedIngresses: []runtime.Object{},
		},
		"diff exists but uid is wrong": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToIngress(superIngress("ing-1", superDefaultNSName, "12345", defaultClusterKey), spec1),
			},
			ExistingObjectInTenant:   applySpecToIngress(tenantIngress("ing-1", "default", "123456"), spec3),
			ExpectedUpdatedIngresses: []runtime.Object{},
			ExpectedError:            "delegated UID is different",
		},
	}
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewIngressController,
				testTenant,
				tc.ExistingObjectInSuper,
				[]runtime.Object{tc.ExistingObjectInTenant},
				tc.ExistingObjectInTenant,
				nil)
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

			if len(tc.ExpectedUpdatedIngresses) != len(actions) {
				t.Errorf("%s: Expected to update ingress %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedIngresses, actions)
				return
			}
			for i, obj := range tc.ExpectedUpdatedIngresses {
				action := actions[i]
				if !action.Matches("update", "ingresses") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				actionObj := action.(core.UpdateAction).GetObject()
				if !equality.Semantic.DeepEqual(obj, actionObj) {
					t.Errorf("%s: Expected updated ingress is %v, got %v", k, obj, actionObj)
				}
			}
		})
	}
}
