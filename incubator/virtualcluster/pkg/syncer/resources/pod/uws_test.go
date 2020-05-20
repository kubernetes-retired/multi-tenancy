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
	"time"

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

func tenantAssignedPod(name, namespace, uid, nodename string) *v1.Pod {
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

func unKnownSuperPod(name, namespace string) *v1.Pod {
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func superAssignedPod(name, namespace, uid, nodename, clusterKey string) *v1.Pod {
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

func fakeNode(name string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				constants.LabelVirtualNode: "true",
			},
		},
	}
}

func applyLabelToPod(pod *v1.Pod, key, value string) *v1.Pod {
	if pod.Labels == nil {
		pod.Labels = make(map[string]string)
	}
	pod.Labels[key] = value
	return pod
}

func applyStatusToPod(pod *v1.Pod, status *v1.PodStatus) *v1.Pod {
	pod.Status = *status.DeepCopy()
	return pod
}

func TestUWPodUpdate(t *testing.T) {
	opaqueMetaPrefix := "foo.bar.super"
	testTenant := &v1alpha1.VirtualCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "tenant-1",
			UID:       "7374a172-c35d-45b1-9c8e-bf5c5b614937",
		},
		Spec: v1alpha1.VirtualClusterSpec{
			TransparentMetaPrefixes: []string{opaqueMetaPrefix},
		},
		Status: v1alpha1.VirtualClusterStatus{
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
				applyStatusToPod(superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey), statusRunning),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", "n1"), statusPending),
				fakeNode("n1"),
			},
			EnquedKey: superDefaultNSName + "/pod-1",
			ExpectedUpdatedPods: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", "n1"), statusRunning),
			},
			ExpectedError: "",
		},
		"update vPod metadata": {
			ExistingObjectInSuper: []runtime.Object{
				applyLabelToPod(applyStatusToPod(superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey), statusRunning), opaqueMetaPrefix+"/a", "b"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", "n1"), statusPending),
				fakeNode("n1"),
			},
			EnquedKey: superDefaultNSName + "/pod-1",
			ExpectedUpdatedPods: []runtime.Object{
				applyLabelToPod(applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", "n1"), statusPending), opaqueMetaPrefix+"/a", "b"),
			},
			ExpectedError: "",
		},
		"vPod existing with different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superAssignedPod("pod-1", superDefaultNSName, "123456", "n1", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantAssignedPod("pod-1", "default", "12345", "n1"),
			},
			EnquedKey:           superDefaultNSName + "/pod-1",
			ExpectedUpdatedPods: []runtime.Object{},
			ExpectedError:       "delegated UID is different",
		},
		"pPod not found": {
			EnquedKey: superDefaultNSName + "/pod-1",
		},
		"pPod not created by syncer": {
			ExistingObjectInSuper: []runtime.Object{
				unKnownSuperPod("pod-1", superDefaultNSName),
			},
			EnquedKey: superDefaultNSName + "/pod-1",
		},
		"vPod not found": {
			ExistingObjectInSuper: []runtime.Object{
				superAssignedPod("pod-1", superDefaultNSName, "123456", "n1", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{},
			EnquedKey:              superDefaultNSName + "/pod-1",
		},
		"vPod not scheduled but super fakeNode is missing": {
			ExistingObjectInSuper: []runtime.Object{
				applyStatusToPod(superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey), statusRunning),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", ""), statusPending),
			},
			EnquedKey:     superDefaultNSName + "/pod-1",
			ExpectedError: "failed to get node",
		},
		"vPod scheduled but vNode not found": {
			ExistingObjectInSuper: []runtime.Object{
				applyStatusToPod(superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey), statusRunning),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", "n1"), statusPending),
			},
			EnquedKey:     superDefaultNSName + "/pod-1",
			ExpectedError: "failed to check vNode",
		},
		//TODO: pod not scheduled case.
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewPodController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnquedKey, nil)
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

			for _, obj := range tc.ExpectedUpdatedPods {
				matched := false
				for _, action := range actions {
					if !action.Matches("update", "pods") {
						continue
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						exp, _ := json.Marshal(obj)
						got, _ := json.Marshal(actionObj)
						t.Errorf("%s: Expected updated pod is %v, got %v", k, string(exp), string(got))
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated pod %+v but not found", k, obj)
				}
			}
		})
	}
}

func TestUWPodDeletion(t *testing.T) {
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

	statusRunning := &v1.PodStatus{
		Phase: v1.PodRunning,
	}

	defaultClusterKey := conversion.ToClusterKey(testTenant)
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		EnquedKey              string
		ExpectedDeletePods     []string
		ExpectedError          string
	}{
		"pPod deleting with grace period and vPod running": {
			ExistingObjectInSuper: []runtime.Object{
				applyDeletionTimestampToPod(applyStatusToPod(superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey), statusRunning), time.Now(), 30),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", "n1"), statusRunning),
				fakeNode("n1"),
			},
			EnquedKey:          superDefaultNSName + "/pod-1",
			ExpectedDeletePods: []string{"default/pod-1"},
			ExpectedError:      "",
		},
		"pPod and vPod both deleting with grace period": {
			ExistingObjectInSuper: []runtime.Object{
				applyDeletionTimestampToPod(applyStatusToPod(superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey), statusRunning), time.Now(), 30),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDeletionTimestampToPod(applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", "n1"), statusRunning), time.Now(), 30),
				fakeNode("n1"),
			},
			EnquedKey:          superDefaultNSName + "/pod-1",
			ExpectedDeletePods: []string{},
			ExpectedError:      "",
		},
		"pPod and vPod both deleting with grace period, but different": {
			ExistingObjectInSuper: []runtime.Object{
				applyDeletionTimestampToPod(applyStatusToPod(superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey), statusRunning), time.Now(), 0),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDeletionTimestampToPod(applyStatusToPod(tenantAssignedPod("pod-1", "default", "12345", "n1"), statusRunning), time.Now(), 30),
				fakeNode("n1"),
			},
			EnquedKey:          superDefaultNSName + "/pod-1",
			ExpectedDeletePods: []string{"default/pod-1"},
			ExpectedError:      "",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewPodController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnquedKey, nil)
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

			for _, expectedName := range tc.ExpectedDeletePods {
				matched := false
				for _, action := range actions {
					if !action.Matches("delete", "pods") {
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, fullName)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect deleted pod %s but not found", k, expectedName)
				}
			}
		})
	}
}
