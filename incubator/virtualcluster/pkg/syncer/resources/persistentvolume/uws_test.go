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
	"encoding/json"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

func TestUWPVUpdate(t *testing.T) {
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
		EnqueuedKey            string
		ExpectedUpdatedObject  []runtime.Object
		ExpectedError          string
	}{
		"pPV not found": {
			ExistingObjectInTenant: []runtime.Object{
				tenantPV("pv", "12345"),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV not ready": {
			ExistingObjectInSuper: []runtime.Object{
				superPV("pv", "12345"),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPV("pv", "12345"),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV bound but pPVC missing": {
			ExistingObjectInSuper: []runtime.Object{
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPV("pv", "12345"),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV bound to a unknown pPVC": {
			ExistingObjectInSuper: []runtime.Object{
				unknownPVC("pvc", superDefaultNSName, "23456"),
				boundPV(unknownPVC("pvc", superDefaultNSName, "23456"), superPV("pv", "12345")),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPV("pv", "12345"),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV exists, vPVC do not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPV("pv", "12345"),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV exists, vPV exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPVC("pvc", superDefaultNSName, "23456"),
				tenantPV("pv", "123456"),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "delegated UID is different",
		},
		"pPV exists, vPV exists with different spec": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				applyPVSourceToPV(boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")), pvSource1),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPVC("pvc", superDefaultNSName, "23456"),
				applyPVSourceToPV(tenantPV("pv", "12345"), pvSource2),
			},
			EnqueuedKey: "pv",
			ExpectedUpdatedObject: []runtime.Object{
				applyPVSourceToPV(tenantPV("pv", "12345"), pvSource1),
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewPVController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey)
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

			for _, obj := range tc.ExpectedUpdatedObject {
				matched := false
				for _, action := range actions {
					if !action.Matches("update", "persistentvolumes") {
						continue
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						exp, _ := json.Marshal(obj)
						got, _ := json.Marshal(actionObj)
						t.Errorf("%s: Expected updated pv is %v, got %v", k, string(exp), string(got))
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated pv %+v but not found", k, obj)
				}
			}
		})
	}
}

func TestUWPVCreation(t *testing.T) {
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
		ExpectedCreatedObject  []string
		ExpectedError          string
	}{
		"pPV not ready": {
			ExistingObjectInSuper: []runtime.Object{
				superPV("pv", "12345"),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV bound but pPVC missing": {
			ExistingObjectInSuper: []runtime.Object{
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV bound to a unknown pPVC": {
			ExistingObjectInSuper: []runtime.Object{
				unknownPVC("pvc", superDefaultNSName, "23456"),
				boundPV(unknownPVC("pvc", superDefaultNSName, "23456"), superPV("pv", "12345")),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV exists, vPVC does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			EnqueuedKey:   "pv",
			ExpectedError: "",
		},
		"pPV exists, vPV does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey),
				boundPV(superPVC("pvc", superDefaultNSName, "23456", defaultClusterKey), superPV("pv", "12345")),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPVC("pvc", "default", "23456"),
			},
			EnqueuedKey: "pv",
			ExpectedCreatedObject: []string{
				"pv",
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewPVController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey)
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

			for _, expectedName := range tc.ExpectedCreatedObject {
				matched := false
				for _, action := range actions {
					if !action.Matches("create", "persistentvolumes") {
						continue
					}
					created := action.(core.CreateAction).GetObject().(*v1.PersistentVolume)
					if created.Name != expectedName {
						t.Errorf("%s: Expected created vPV %s, got %s", k, expectedName, created.Name)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated pv %+v but not found", k, expectedName)
				}
			}
		})
	}
}
