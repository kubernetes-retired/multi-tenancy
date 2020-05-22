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

package event

import (
	"encoding/json"
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

func superNamespace(name, clusterKey, tenantNamespace string) *v1.Namespace {
	ns := &v1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if clusterKey != "" {
		ns.Annotations = map[string]string{
			constants.LabelCluster:   clusterKey,
			constants.LabelNamespace: tenantNamespace,
		}
	}

	return ns
}

func fakeEvent(name, namespace string, involvedObject v1.ObjectReference) *v1.Event {
	return &v1.Event{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Event",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		InvolvedObject: involvedObject,
		Message:        "test",
	}
}

func tenantPod(name, namespace, uid string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func tenantService(name, namespace, uid string) *v1.Service {
	return &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func makeObjectReference(kind, namespace, name, uid string) v1.ObjectReference {
	return v1.ObjectReference{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		UID:       types.UID(uid),
	}
}

func TestUWEvent(t *testing.T) {
	testTenant := &v1alpha1.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "tenant-1",
			UID:       "7374a172-c35d-45b1-9c8e-bf5c5b614937",
		},
		Status: v1alpha1.VirtualClusterStatus{
			Phase: v1alpha1.ClusterRunning,
		},
	}

	defaultClusterKey := conversion.ToClusterKey(testTenant)
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		EnqueuedKey            string
		ExpectedCreatedObject  []runtime.Object
		ExpectedNoOperation    bool
		ExpectedError          string
	}{
		"pEvent not found": {
			EnqueuedKey:         superDefaultNSName + "/event",
			ExpectedNoOperation: true,
		},
		"pEvent not related to tenant and ns not found(actually impossible)": {
			ExistingObjectInSuper: []runtime.Object{
				fakeEvent("event", superDefaultNSName, makeObjectReference("Pod", superDefaultNSName, "pod", "23456")),
			},
			EnqueuedKey:         superDefaultNSName + "/event",
			ExpectedNoOperation: true,
		},
		"pEvent not related to tenant": {
			ExistingObjectInSuper: []runtime.Object{
				fakeEvent("event", superDefaultNSName, makeObjectReference("Pod", superDefaultNSName, "pod", "23456")),
				superNamespace(superDefaultNSName, "", ""),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod", "default", "12345"),
			},
			EnqueuedKey:         superDefaultNSName + "/event",
			ExpectedNoOperation: true,
		},
		"pEvent exists but vPod doesn't exists": {
			ExistingObjectInSuper: []runtime.Object{
				fakeEvent("event", superDefaultNSName, makeObjectReference("Pod", superDefaultNSName, "pod", "23456")),
				superNamespace(superDefaultNSName, defaultClusterKey, "default"),
			},
			EnqueuedKey:         superDefaultNSName + "/event",
			ExpectedNoOperation: true,
		},
		"pEvent exists but not an accepted event": {
			ExistingObjectInSuper: []runtime.Object{
				fakeEvent("event", superDefaultNSName, makeObjectReference("ConfigMap", superDefaultNSName, "cm", "23456")),
				superNamespace(superDefaultNSName, defaultClusterKey, "default"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod", "default", "12345"),
			},
			EnqueuedKey:         superDefaultNSName + "/event",
			ExpectedNoOperation: true,
		},
		"pEvent exists but vEvent doesn't exists, type pod": {
			ExistingObjectInSuper: []runtime.Object{
				fakeEvent("event", superDefaultNSName, makeObjectReference("Pod", superDefaultNSName, "pod", "23456")),
				superNamespace(superDefaultNSName, defaultClusterKey, "default"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod", "default", "12345"),
			},
			EnqueuedKey: superDefaultNSName + "/event",
			ExpectedCreatedObject: []runtime.Object{
				fakeEvent("event", "default", makeObjectReference("Pod", "default", "pod", "12345")),
			},
		},
		"pEvent exists but vEvent doesn't exists, type service": {
			ExistingObjectInSuper: []runtime.Object{
				fakeEvent("event", superDefaultNSName, makeObjectReference("Service", superDefaultNSName, "svc", "23456")),
				superNamespace(superDefaultNSName, defaultClusterKey, "default"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantService("svc", "default", "12345"),
			},
			EnqueuedKey: superDefaultNSName + "/event",
			ExpectedCreatedObject: []runtime.Object{
				fakeEvent("event", "default", makeObjectReference("Service", "default", "svc", "12345")),
			},
		},
		"pEvent exists and vEvent exists": {
			ExistingObjectInSuper: []runtime.Object{
				fakeEvent("event", superDefaultNSName, makeObjectReference("Pod", superDefaultNSName, "pod", "23456")),
				superNamespace(superDefaultNSName, defaultClusterKey, "default"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod", "default", "12345"),
				fakeEvent("event", "default", makeObjectReference("Pod", "default", "pod", "12345")),
			},
			EnqueuedKey:         superDefaultNSName + "/event",
			ExpectedNoOperation: true,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewEventController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, nil)
			if err != nil {
				t.Errorf("%s: error running upward sync: %v", k, err)
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

			for _, obj := range tc.ExpectedCreatedObject {
				matched := false
				for _, action := range actions {
					if !action.Matches("create", "Events") {
						continue
					}
					actionObj := action.(core.CreateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						exp, _ := json.Marshal(obj)
						got, _ := json.Marshal(actionObj)
						t.Errorf("%s: Expected created Event is %v, got %v", k, string(exp), string(got))
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect created Event %+v but not found", k, obj)
				}
			}
		})
	}
}
