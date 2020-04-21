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

package persistentvolume

import (
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	core "k8s.io/client-go/testing"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func superPV(name, uid string) *v1.PersistentVolume {
	return &v1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(uid),
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: map[v1.ResourceName]resource.Quantity{
				v1.ResourceStorage: resource.MustParse("20Gi"),
			},
			StorageClassName: "storage-class-1",
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

func boundPV(pvc *v1.PersistentVolumeClaim, pv *v1.PersistentVolume) *v1.PersistentVolume {
	pv.Spec.ClaimRef = &v1.ObjectReference{
		Kind:            "PersistentVolumeClaim",
		Namespace:       pvc.Namespace,
		Name:            pvc.Name,
		UID:             pvc.UID,
		APIVersion:      "v1",
		ResourceVersion: "123",
	}
	return pv
}

func unknownPVC(name, namespace, uid string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func tenantPV(name, uid string) *v1.PersistentVolume {
	return &v1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				constants.LabelUID: uid,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			Capacity: map[v1.ResourceName]resource.Quantity{
				v1.ResourceStorage: resource.MustParse("20Gi"),
			},
			StorageClassName: "storage-class-1",
		},
	}
}

func tenantPVC(name, namespace, uid string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
	}
}

func applyPVSourceToPV(pv *v1.PersistentVolume, pvs *v1.PersistentVolumeSource) *v1.PersistentVolume {
	pv.Spec.PersistentVolumeSource = *pvs.DeepCopy()
	return pv
}

func TestPVPatrol(t *testing.T) {
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

	pvSource1 := &v1.PersistentVolumeSource{
		CSI: &v1.CSIPersistentVolumeSource{
			Driver:       "csi-driver-1",
			VolumeHandle: "d-volume1",
			FSType:       "ext4",
		},
	}

	pvSource2 := &v1.PersistentVolumeSource{
		CSI: &v1.CSIPersistentVolumeSource{
			Driver:       "csi-driver2",
			VolumeHandle: "d-volume1",
			FSType:       "ext4",
		},
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedDeletedPObject []string
		ExpectedDeletedVObject []string
		ExpectedCreatedVObject []string
		ExpectedUpdatedPObject []runtime.Object
		ExpectedUpdatedVObject []runtime.Object
		WaitDWS                bool // Make sure to set this flag if the test involves DWS.
		WaitUWS                bool // Make sure to set this flag if the test involves UWS.
	}{
		"pPV not bound": {
			ExistingObjectInSuper: []runtime.Object{
				superPV("pv", "12345"),
			},
		},
		"pPV bound but pPVC missing": {
			ExistingObjectInSuper: []runtime.Object{
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
		},
		"pPVC not created by vc": {
			ExistingObjectInSuper: []runtime.Object{
				unknownPVC("pvc", superDefaultNSName, "23456"),
				boundPV(unknownPVC("pvc", superDefaultNSName, "23456"), superPV("pv", "12345")),
			},
		},
		"pPV exists, vPV and vPVC does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			WaitUWS: true,
		},
		"pPV exists, vPV does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPVC("pvc", "default", "23456"),
			},
			ExpectedCreatedVObject: []string{
				"/pv",
			},
			WaitUWS: true,
		},
		"pPV exists, vPV exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPV("pv", "11111"),
			},
			ExpectedDeletedVObject: []string{
				"/pv",
			},
		},
		"vPV exists, pPV exists with different spec": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				applyPVSourceToPV(boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")), pvSource1),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyPVSourceToPV(tenantPV("pv", "12345"), pvSource2),
			},
			ExpectedUpdatedVObject: []runtime.Object{
				applyPVSourceToPV(tenantPV("pv", "12345"), pvSource1),
			},
			WaitUWS: true,
		},
		"vPV exists, pPV does not exists": {
			ExistingObjectInTenant: []runtime.Object{
				tenantPV("pv", "12345"),
			},
			ExpectedDeletedVObject: []string{
				"/pv",
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tenantActions, superActions, err := util.RunPatrol(NewPVController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.WaitDWS, tc.WaitUWS, nil)
			if err != nil {
				t.Errorf("%s: error running patrol: %v", k, err)
				return
			}

			if tc.ExpectedDeletedPObject != nil {
				if len(tc.ExpectedDeletedPObject) != len(superActions) {
					t.Errorf("%s: Expected to delete pPVC %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedPObject {
					action := superActions[i]
					if !action.Matches("delete", "persistentvolumes") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pPVC %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedDeletedVObject != nil {
				if len(tc.ExpectedDeletedVObject) != len(tenantActions) {
					t.Errorf("%s: Expected to delete VPVC %#v. Actual actions were: %#v", k, tc.ExpectedDeletedVObject, tenantActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedVObject {
					action := tenantActions[i]
					if !action.Matches("delete", "persistentvolumes") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pPVC %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedCreatedVObject != nil {
				if len(tc.ExpectedCreatedVObject) != len(tenantActions) {
					t.Errorf("%s: Expected to create vPVC %#v. Actual actions were: %#v", k, tc.ExpectedCreatedVObject, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedCreatedVObject {
					action := tenantActions[i]
					if !action.Matches("create", "persistentvolumes") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					created := action.(core.CreateAction).GetObject().(*v1.PersistentVolume)
					fullName := created.Namespace + "/" + created.Name
					if fullName != expectedName {
						t.Errorf("%s: Expect to create pPVC %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedUpdatedPObject != nil {
				if len(tc.ExpectedUpdatedPObject) != len(superActions) {
					t.Errorf("%s: Expected to update PPVC %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPObject, superActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedPObject {
					action := superActions[i]
					if !action.Matches("update", "persistentvolumes") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated pPVC is %v, got %v", k, obj, actionObj)
					}
				}
			}
			if tc.ExpectedUpdatedVObject != nil {
				if len(tc.ExpectedUpdatedVObject) != len(tenantActions) {
					t.Errorf("%s: Expected to update VPVC %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedVObject, tenantActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedVObject {
					action := tenantActions[i]
					if !action.Matches("update", "persistentvolumes") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated vPVC is %v, got %v", k, obj, actionObj)
					}
				}
			}
		})
	}
}
