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

package endpoints

import (
	"encoding/json"
	"fmt"
	"k8s.io/utils/pointer"
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

func tenantEndpoints(name, namespace, uid string) *v1.Endpoints {
	return &v1.Endpoints{
		TypeMeta: metav1.TypeMeta{
			Kind:       "v1",
			APIVersion: "Endpoints",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func superEndpoints(name, namespace, uid, clusterKey string) *v1.Endpoints {
	return &v1.Endpoints{
		TypeMeta: metav1.TypeMeta{
			Kind:       "v1",
			APIVersion: "Endpoints",
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

func tenantService(name, namespace, uid string) *v1.Service {
	return &v1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Spec: v1.ServiceSpec{
			Selector: nil,
		},
	}
}

func applySelectorToService(svc *v1.Service, key, value string) *v1.Service {
	svc.Spec.Selector = map[string]string{
		key: value,
	}
	return svc
}

func TestDWEndpointsCreation(t *testing.T) {
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
		ExpectedCreatedPObject []string
		ExpectedNoOperation    bool
		ExpectedError          string
	}{
		"new ep": {
			ExistingObjectInSuper: []runtime.Object{},
			ExistingObjectInTenant: []runtime.Object{
				tenantEndpoints("svc", "default", "12345"),
			},
			ExpectedCreatedPObject: []string{superDefaultNSName + "/svc"},
		},
		"new ep but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superEndpoints("svc", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantEndpoints("svc", "default", "12345"),
			},
			ExpectedNoOperation: true,
		},
		"new ep but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superEndpoints("svc", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantEndpoints("svc", "default", "12345"),
			},
			ExpectedError: "delegated UID is different",
		},
		"new ep related to service without selector": {
			ExistingObjectInTenant: []runtime.Object{
				tenantEndpoints("svc", "default", "12345"),
				tenantService("svc", "default", "123456"),
			},
			ExpectedCreatedPObject: []string{superDefaultNSName + "/svc"},
		},
		"new ep related to service with selector": {
			ExistingObjectInTenant: []runtime.Object{
				tenantEndpoints("svc", "default", "12345"),
				applySelectorToService(tenantService("svc", "default", "123456"), "a", "b"),
			},
			ExpectedNoOperation: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewEndpointsController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], nil)
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
				t.Errorf("%s: Expected to create ep %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPObject, actions)
				return
			}
			for i, expectedName := range tc.ExpectedCreatedPObject {
				action := actions[i]
				if !action.Matches("create", "endpoints") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				created := action.(core.CreateAction).GetObject().(*v1.Endpoints)
				fullName := created.Namespace + "/" + created.Name
				if fullName != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, fullName)
				}
			}
		})
	}
}

func TestDWEndpointsDeletion(t *testing.T) {
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
		EnqueueObject          *v1.Endpoints
		ExpectedDeletedPObject []string
		ExpectedNoOperation    bool
		ExpectedError          string
	}{
		"delete ep": {
			ExistingObjectInSuper: []runtime.Object{
				superEndpoints("svc", superDefaultNSName, "12345", defaultClusterKey),
			},
			EnqueueObject:          tenantEndpoints("svc", "default", "12345"),
			ExpectedDeletedPObject: []string{superDefaultNSName + "/svc"},
		},
		"delete ep but already gone": {
			ExistingObjectInSuper: []runtime.Object{},
			EnqueueObject:         tenantEndpoints("svc", "default", "12345"),
			ExpectedNoOperation:   true,
		},
		"delete ep but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superEndpoints("svc", superDefaultNSName, "123456", defaultClusterKey),
			},
			EnqueueObject: tenantEndpoints("svc", "default", "12345"),
			ExpectedError: "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewEndpointsController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueueObject, nil)
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
				t.Errorf("%s: Expected to delete ep %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPObject, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedPObject {
				action := actions[i]
				if !action.Matches("delete", "endpoints") {
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

func applySpecToEndpoints(ep *v1.Endpoints, sbs []v1.EndpointSubset) *v1.Endpoints {
	ep.Subsets = sbs
	return ep
}

func TestDWEndpointsUpdate(t *testing.T) {
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

	subset1 := []v1.EndpointSubset{
		{
			Addresses: []v1.EndpointAddress{
				{
					IP:       "1.1.1.1",
					NodeName: pointer.StringPtr("n1"),
					TargetRef: &v1.ObjectReference{
						Kind:      "Pod",
						Namespace: "n1",
						Name:      "pod1",
						UID:       "12345",
					},
				},
			},
		},
	}

	subset2 := []v1.EndpointSubset{
		{
			Addresses: []v1.EndpointAddress{
				{
					IP:       "1.1.1.1",
					NodeName: pointer.StringPtr("n1"),
					TargetRef: &v1.ObjectReference{
						Kind:      "Pod",
						Namespace: "n2",
						Name:      "pod1",
						UID:       "123456",
					},
				},
			},
		},
	}

	subset3 := []v1.EndpointSubset{
		{
			Addresses: []v1.EndpointAddress{
				{
					IP:       "1.1.1.2",
					NodeName: pointer.StringPtr("n2"),
					TargetRef: &v1.ObjectReference{
						Kind:      "Pod",
						Namespace: "n1",
						Name:      "pod1",
						UID:       "12345",
					},
				},
			},
		},
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedUpdatedPObject []runtime.Object
		ExpectedNoOperation    bool
		ExpectedError          string
	}{
		"no diff": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToEndpoints(superEndpoints("svc", superDefaultNSName, "12345", defaultClusterKey), subset1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToEndpoints(tenantEndpoints("svc", "default", "12345"), subset1),
			},
			ExpectedNoOperation: true,
		},
		"diff in targetRef": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToEndpoints(superEndpoints("svc", superDefaultNSName, "12345", defaultClusterKey), subset1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToEndpoints(tenantEndpoints("svc", "default", "12345"), subset2),
			},
			ExpectedNoOperation: true,
		},
		//"diff in subset address": {
		//	ExistingObjectInSuper: []runtime.Object{
		//		applySpecToEndpoints(superEndpoints("svc", superDefaultNSName, "12345", defaultClusterKey), subset1),
		//	},
		//	ExistingObjectInTenant: []runtime.Object{
		//		applySpecToEndpoints(tenantEndpoints("svc", "default", "12345"), subset3),
		//	},
		//	ExpectedUpdatedPObject: []runtime.Object{
		//		applySpecToEndpoints(superEndpoints("svc", superDefaultNSName, "12345", defaultClusterKey), subset3),
		//	},
		//},
		"diff in uid": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToEndpoints(superEndpoints("svc", superDefaultNSName, "12345", defaultClusterKey), subset1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToEndpoints(tenantEndpoints("svc", "default", "123456"), subset3),
			},
			ExpectedError:       "delegated UID is different",
			ExpectedNoOperation: true,
		},
	}
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewEndpointsController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], nil)
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
				t.Errorf("%s: Expected to update ep %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPObject, actions)
				return
			}
			for i, obj := range tc.ExpectedUpdatedPObject {
				action := actions[i]
				if !action.Matches("update", "endpoints") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				actionObj := action.(core.UpdateAction).GetObject()
				bb, _ := json.Marshal(actionObj)
				fmt.Println(string(bb))
				bb, _ = json.Marshal(obj)
				fmt.Println(string(bb))
				if !equality.Semantic.DeepEqual(obj, actionObj) {
					t.Errorf("%s: Expected updated ep is %v, got %v", k, obj, actionObj)
				}
			}
		})
	}
}
