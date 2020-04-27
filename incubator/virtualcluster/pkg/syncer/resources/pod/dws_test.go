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
	"strings"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/testing"
	"k8s.io/utils/pointer"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func tenantPod(name, namespace, uid string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func applyNodeNameToPod(vPod *v1.Pod, nodeName string) *v1.Pod {
	vPod.Spec.NodeName = nodeName
	return vPod
}

func applyDeletionTimestampToPod(vPod *v1.Pod, t time.Time, gracePeriodSeconds int64) *v1.Pod {
	metaTime := metav1.NewTime(t)
	vPod.DeletionTimestamp = &metaTime
	vPod.DeletionGracePeriodSeconds = pointer.Int64Ptr(gracePeriodSeconds)
	return vPod
}

func superPod(name, namespace, uid string) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				constants.LabelUID: uid,
			},
		},
	}
}

func tenantServiceAccount(name, namespace, uid string) *v1.ServiceAccount {
	return &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Secrets: []v1.ObjectReference{
			{
				Name: "default-token-x6nbf",
			},
		},
	}
}

func superService(name, namespace, uid string, clusterIP string) *v1.Service {
	svc := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				constants.LabelUID: uid,
			},
		},
	}
	if clusterIP != "" {
		svc.Spec.ClusterIP = clusterIP
	}
	return svc
}

func superSecret(name, namespace, uid string) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelServiceAccountUID: uid,
			},
		},
	}
}

func TestDWPodCreation(t *testing.T) {
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
		ExpectedCreatedPods    []string
		ExpectedError          string
	}{
		"new Pod": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("default-token-12345", superDefaultNSName, "12345"),
				superService("kubernetes", superDefaultNSName, "12345", ""),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod-1", "default", "12345"),
				tenantServiceAccount("default", "default", "12345"),
			},
			ExpectedCreatedPods: []string{superDefaultNSName + "/pod-1"},
		},
		"load pod which under deletion": {
			ExistingObjectInSuper: []runtime.Object{},
			ExistingObjectInTenant: []runtime.Object{
				applyDeletionTimestampToPod(tenantPod("pod-1", "default", "12345"), time.Now(), 30),
			},
			ExpectedCreatedPods: []string{},
			ExpectedError:       "",
		},
		"missing service account token secret": {
			ExistingObjectInSuper: []runtime.Object{
				superService("kubernetes", superDefaultNSName, "12345", ""),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod-1", "default", "12345"),
				tenantServiceAccount("default", "default", "12345"),
			},
			ExpectedError: "service account token secret for pod is not ready",
		},
		"without any services": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("default-token-12345", superDefaultNSName, "12345"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod-1", "default", "12345"),
				tenantServiceAccount("default", "default", "12345"),
			},
			ExpectedError: "service is not ready",
		},
		"only a dns service": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("default-token-12345", conversion.ToSuperMasterNamespace(defaultClusterKey, "kube-system"), "12345"),
				superService(constants.TenantDNSServerServiceName, conversion.ToSuperMasterNamespace(defaultClusterKey, constants.TenantDNSServerNS), "12345", "192.168.0.10"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod-1", "kube-system", "12345"),
				tenantServiceAccount("default", "kube-system", "12345"),
			},
			ExpectedCreatedPods: []string{conversion.ToSuperMasterNamespace(defaultClusterKey, "kube-system") + "/pod-1"},
			ExpectedError:       "",
		},
		"new pod with nodeName": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("default-token-12345", superDefaultNSName, "12345"),
				superService("kubernetes", superDefaultNSName, "12345", ""),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyNodeNameToPod(tenantPod("pod-1", "default", "12345"), "i-xxxx"),
				tenantServiceAccount("default", "default", "12345"),
			},
			ExpectedCreatedPods: []string{},
		},
		"new Pod but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superPod("pod-1", superDefaultNSName, "12345"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod-1", "default", "12345"),
			},
			ExpectedCreatedPods: []string{},
			ExpectedError:       "",
		},
		"new Pod but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superPod("pod-1", superDefaultNSName, "123456"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod-1", "default", "12345"),
			},
			ExpectedCreatedPods: []string{},
			ExpectedError:       "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewPodController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], nil)
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

			if len(tc.ExpectedCreatedPods) != len(actions) {
				t.Errorf("%s: Expected to create Pod %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPods, actions)
				return
			}
			for i, expectedName := range tc.ExpectedCreatedPods {
				action := actions[i]
				if !action.Matches("create", "pods") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				createdPod := action.(core.CreateAction).GetObject().(*v1.Pod)
				fullName := createdPod.Namespace + "/" + createdPod.Name
				if fullName != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, fullName)
				}
			}
		})
	}
}

func TestDWPodDeletion(t *testing.T) {
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
		EnqueueObject          *v1.Pod
		ExpectedDeletedPods    []string
		ExpectedError          string
	}{
		"delete Pod": {
			ExistingObjectInSuper: []runtime.Object{
				superPod("pod-1", superDefaultNSName, "12345"),
			},
			EnqueueObject:       tenantPod("pod-1", "default", "12345"),
			ExpectedDeletedPods: []string{superDefaultNSName + "/pod-1"},
		},
		"delete vPod and pPod is already running": {
			ExistingObjectInSuper: []runtime.Object{
				applyNodeNameToPod(superPod("pod-1", superDefaultNSName, "12345"), "i-xxx"),
			},
			EnqueueObject:       tenantPod("pod-1", "default", "12345"),
			ExpectedDeletedPods: []string{superDefaultNSName + "/pod-1"},
		},
		"delete Pod but already gone": {
			ExistingObjectInSuper: []runtime.Object{},
			EnqueueObject:         tenantPod("pod-1", "default", "12345"),
			ExpectedDeletedPods:   []string{},
			ExpectedError:         "",
		},
		"delete Pod but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superPod("pod-1", superDefaultNSName, "123456"),
			},
			EnqueueObject:       tenantPod("pod-1", "default", "12345"),
			ExpectedDeletedPods: []string{},
			ExpectedError:       "delegated UID is different",
		},
		"terminating vPod but running pPod": {
			ExistingObjectInSuper: []runtime.Object{
				superPod("pod-1", superDefaultNSName, "12345"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDeletionTimestampToPod(tenantPod("pod-1", "default", "12345"), time.Now(), 30),
			},
			EnqueueObject:       applyDeletionTimestampToPod(tenantPod("pod-1", "default", "12345"), time.Now(), 30),
			ExpectedDeletedPods: []string{superDefaultNSName + "/pod-1"},
			ExpectedError:       "",
		},
		"terminating vPod and terminating pPod": {
			ExistingObjectInSuper: []runtime.Object{
				applyDeletionTimestampToPod(superPod("pod-1", superDefaultNSName, "12345"), time.Now(), 30),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyDeletionTimestampToPod(tenantPod("pod-1", "default", "12345"), time.Now(), 30),
			},
			EnqueueObject:       applyDeletionTimestampToPod(tenantPod("pod-1", "default", "12345"), time.Now(), 30),
			ExpectedDeletedPods: []string{},
			ExpectedError:       "",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewPodController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueueObject, nil)
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

			if len(tc.ExpectedDeletedPods) != len(actions) {
				t.Errorf("%s: Expected to delete pod %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPods, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedPods {
				action := actions[i]
				if !action.Matches("delete", "pods") {
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

func applySpecToPod(pod *v1.Pod, spec *v1.PodSpec) *v1.Pod {
	pod.Spec = *spec.DeepCopy()
	return pod
}

func TestDWPodUpdate(t *testing.T) {
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

	spec1 := &v1.PodSpec{
		Containers: []v1.Container{
			{
				Image: "ngnix",
				Name:  "c-1",
			},
		},
		NodeName: "i-xxx",
	}

	spec2 := &v1.PodSpec{
		Containers: []v1.Container{
			{
				Image: "busybox",
				Name:  "c-1",
			},
		},
		NodeName: "i-xxx",
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedUpdatedPods    []runtime.Object
		ExpectedError          string
	}{
		"no diff": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToPod(superPod("pod-1", superDefaultNSName, "12345"), spec1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToPod(tenantPod("pod-1", "default", "12345"), spec1),
			},
			ExpectedUpdatedPods: []runtime.Object{},
		},
		"diff in container": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToPod(superPod("pod-1", superDefaultNSName, "12345"), spec1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToPod(tenantPod("pod-1", "default", "12345"), spec2),
			},
			ExpectedUpdatedPods: []runtime.Object{
				applySpecToPod(superPod("pod-1", superDefaultNSName, "12345"), spec2),
			},
		},
		"diff exists but uid is wrong": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToPod(superPod("pod-1", superDefaultNSName, "12345"), spec1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToPod(tenantPod("pod-1", "default", "123456"), spec2),
			},
			ExpectedUpdatedPods: []runtime.Object{},
			ExpectedError:       "delegated UID is different",
		},
	}
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewPodController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], nil)
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

			if len(tc.ExpectedUpdatedPods) != len(actions) {
				t.Errorf("%s: Expected to update pod %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPods, actions)
				return
			}
			for i, obj := range tc.ExpectedUpdatedPods {
				action := actions[i]
				if !action.Matches("update", "pods") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				actionObj := action.(core.UpdateAction).GetObject()
				if !equality.Semantic.DeepEqual(obj, actionObj) {
					t.Errorf("%s: Expected updated pod is %v, got %v", k, obj, actionObj)
				}
			}
		})
	}
}
