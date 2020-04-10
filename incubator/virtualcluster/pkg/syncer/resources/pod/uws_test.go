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

package pod

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

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func uwTenantPod(name, namespace, uid, nodename string) *v1.Pod {
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Spec: v1.PodSpec{
			NodeName: nodename,
		},
	}
}

func uwSuperPod(name, namespace, uid, nodename, clusterKey string) *v1.Pod {
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
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
		Spec: v1.PodSpec{
			NodeName: nodename,
		},
	}
}

func tenantNode(name string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

func applyStatusToPod(pod *v1.Pod, status *v1.PodStatus) *v1.Pod {
	pod.Status = *status.DeepCopy()
	return pod
}

func TestUWPodUpdate(t *testing.T) {
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

	statusPending := &v1.PodStatus{
		Phase: "Pending",
	}

	statusRunning := &v1.PodStatus{
		Phase: "Running",
	}

	defaultClusterKey := conversion.ToClusterKey(testTenant)
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		EnquedKey              string
		ExpectedUpdatedPods    []runtime.Object
		ExpectedError          string
	}{
		"update vPod status": {
			ExistingObjectInSuper: []runtime.Object{
				applyStatusToPod(uwSuperPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey), statusRunning),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(uwTenantPod("pod-1", "default", "12345", "n1"), statusPending),
				tenantNode("n1"),
			},
			EnquedKey: superDefaultNSName + "/pod-1",
			ExpectedUpdatedPods: []runtime.Object{
				applyStatusToPod(uwTenantPod("pod-1", "default", "12345", "n1"), statusRunning),
			},
			ExpectedError: "",
		},
		"vPod existing with different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				uwSuperPod("pod-1", superDefaultNSName, "123456", "n1", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				uwTenantPod("pod-1", "default", "12345", "n1"),
			},
			EnquedKey:           superDefaultNSName + "/pod-1",
			ExpectedUpdatedPods: []runtime.Object{},
			ExpectedError:       "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewPodController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnquedKey)
			if err != nil {
				t.Errorf("%s: error running upward sync: %v", k, err)
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

			if len(tc.ExpectedUpdatedPods) != len(actions) {
				t.Errorf("%s: Expected to update Pod to %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPods, actions)
				return
			}
			for i, obj := range tc.ExpectedUpdatedPods {
				action := actions[i]
				if !action.Matches("update", "pods") {
					t.Errorf("%s: Unexpected action %s", k, action)
					continue
				}
				actionObj := action.(core.UpdateAction).GetObject()
				if !equality.Semantic.DeepEqual(obj, actionObj) {
					exp, _ := json.Marshal(obj)
					got, _ := json.Marshal(actionObj)
					t.Errorf("%s: Expected updated pod is %v, got %v", k, string(exp), string(got))
				}
			}
		})
	}
}
