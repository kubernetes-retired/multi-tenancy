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

package persistentvolumeclaim

import (
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
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

func tenantPVC(name, namespace, uid string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func superPVC(name, namespace, uid, clusterKey string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
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

func unknownPVC(name, namespace string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func TestDWPVCCreation(t *testing.T) {
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
		ExpectedCreatedPVC     []string
		ExpectedError          string
	}{
		"new pvc": {
			ExistingObjectInSuper: []runtime.Object{},
			ExistingObjectInTenant: []runtime.Object{
				tenantPVC("pvc-1", "default", "12345"),
			},
			ExpectedCreatedPVC: []string{superDefaultNSName + "/pvc-1"},
		},
		"new pvc but already exists": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc-1", superDefaultNSName, "12345", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPVC("pvc-1", "default", "12345"),
			},
			ExpectedCreatedPVC: []string{},
			ExpectedError:      "",
		},
		"new pvc but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc-1", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPVC("pvc-1", "default", "12345"),
			},
			ExpectedCreatedPVC: []string{},
			ExpectedError:      "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewPVCController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], nil)
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

			if len(tc.ExpectedCreatedPVC) != len(actions) {
				t.Errorf("%s: Expected to create PVC %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPVC, actions)
				return
			}
			for i, expectedName := range tc.ExpectedCreatedPVC {
				action := actions[i]
				if !action.Matches("create", "persistentvolumeclaims") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				created := action.(core.CreateAction).GetObject().(*v1.PersistentVolumeClaim)
				fullName := created.Namespace + "/" + created.Name
				if fullName != expectedName {
					t.Errorf("%s: Expected %s to be created, got %s", k, expectedName, fullName)
				}
			}
		})
	}
}

func TestDWPVCDeletion(t *testing.T) {
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
		EnqueueObject          *v1.PersistentVolumeClaim
		ExpectedDeletedPVC     []string
		ExpectedError          string
	}{
		"delete pvc": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc-1", superDefaultNSName, "12345", defaultClusterKey),
			},
			EnqueueObject:      tenantPVC("pvc-1", "default", "12345"),
			ExpectedDeletedPVC: []string{superDefaultNSName + "/pvc-1"},
		},
		"delete pvc but already gone": {
			ExistingObjectInSuper: []runtime.Object{},
			EnqueueObject:         tenantPVC("pvc-1", "default", "12345"),
			ExpectedDeletedPVC:    []string{},
			ExpectedError:         "",
		},
		"delete pvc but existing different uid one": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc-1", superDefaultNSName, "123456", defaultClusterKey),
			},
			EnqueueObject:      tenantPVC("pvc-1", "default", "12345"),
			ExpectedDeletedPVC: []string{},
			ExpectedError:      "delegated UID is different",
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewPVCController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueueObject, nil)
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

			if len(tc.ExpectedDeletedPVC) != len(actions) {
				t.Errorf("%s: Expected to delete pvc %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPVC, actions)
				return
			}
			for i, expectedName := range tc.ExpectedDeletedPVC {
				action := actions[i]
				if !action.Matches("delete", "persistentvolumeclaims") {
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

func applySpecToPVC(pvc *v1.PersistentVolumeClaim, spec *v1.PersistentVolumeClaimSpec) *v1.PersistentVolumeClaim {
	pvc.Spec = *spec.DeepCopy()
	return pvc
}

func TestDWPVCUpdate(t *testing.T) {
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

	spec1 := &v1.PersistentVolumeClaimSpec{
		AccessModes: []v1.PersistentVolumeAccessMode{
			v1.ReadWriteOnce,
		},
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse("20Gi"),
			},
		},
		StorageClassName: pointer.StringPtr("storage-class-1"),
		VolumeName:       "volume-1",
	}

	spec2 := &v1.PersistentVolumeClaimSpec{
		AccessModes: []v1.PersistentVolumeAccessMode{
			v1.ReadWriteOnce,
		},
		Resources: v1.ResourceRequirements{
			Requests: v1.ResourceList{
				v1.ResourceStorage: resource.MustParse("30Gi"),
			},
		},
		StorageClassName: pointer.StringPtr("storage-class-1"),
		VolumeName:       "volume-1",
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedUpdatedPVC     []runtime.Object
		ExpectedError          string
	}{
		"no diff": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToPVC(superPVC("pvc-1", superDefaultNSName, "12345", defaultClusterKey), spec1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToPVC(tenantPVC("pvc-1", "default", "12345"), spec1),
			},
			ExpectedUpdatedPVC: []runtime.Object{},
		},
		"diff in storage size": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToPVC(superPVC("pvc-1", superDefaultNSName, "12345", defaultClusterKey), spec1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToPVC(tenantPVC("pvc-1", "default", "12345"), spec2),
			},
			ExpectedUpdatedPVC: []runtime.Object{
				applySpecToPVC(superPVC("pvc-1", superDefaultNSName, "12345", defaultClusterKey), spec2),
			},
		},
		"diff exists but uid is wrong": {
			ExistingObjectInSuper: []runtime.Object{
				applySpecToPVC(superPVC("pvc-1", superDefaultNSName, "12345", defaultClusterKey), spec1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applySpecToPVC(tenantPVC("pvc-1", "default", "123456"), spec2),
			},
			ExpectedUpdatedPVC: []runtime.Object{},
			ExpectedError:      "delegated UID is different",
		},
	}
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunDownwardSync(NewPVCController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.ExistingObjectInTenant[0], nil)
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

			if len(tc.ExpectedUpdatedPVC) != len(actions) {
				t.Errorf("%s: Expected to update pvc %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPVC, actions)
				return
			}
			for i, obj := range tc.ExpectedUpdatedPVC {
				action := actions[i]
				if !action.Matches("update", "persistentvolumeclaims") {
					t.Errorf("%s: Unexpected action %s", k, action)
				}
				actionObj := action.(core.UpdateAction).GetObject()
				if !equality.Semantic.DeepEqual(obj, actionObj) {
					t.Errorf("%s: Expected updated pvc is %v, got %v", k, obj, actionObj)
				}
			}
		})
	}
}
