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
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func TestPodPatrol(t *testing.T) {
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
	spec1 := &v1.PodSpec{
		Containers: []v1.Container{
			{
				Image: "ngnix",
				Name:  "c-1",
			},
		},
		NodeName: "n1",
	}
	spec2 := &v1.PodSpec{
		Containers: []v1.Container{
			{
				Image: "busybox",
				Name:  "c-1",
			},
		},
		NodeName: "n1",
	}
	statusPending := &v1.PodStatus{
		Phase: "Pending",
	}
	statusReadyAndRunning := &v1.PodStatus{
		Phase: "Running",
		Conditions: []v1.PodCondition{
			{
				Type:   "PodScheduled",
				Status: "True",
			},
		},
	}

	defaultClusterKey := conversion.ToClusterKey(testTenant)
	superDefaultNSName := conversion.ToSuperMasterNamespace(defaultClusterKey, "default")

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedDeletedPPods   []string
		ExpectedDeletedVPods   []string
		ExpectedCreatedPPods   []string
		ExpectedUpdatedPPods   []runtime.Object
		ExpectedUpdatedVPods   []runtime.Object
		WaitDWS                bool // Make sure to set this flag if the test involves DWS.
		WaitUWS                bool // Make sure to set this flag if the test involves UWS.
	}{
		"pPod exists, vPod does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey),
			},
			ExpectedDeletedPPods: []string{
				superDefaultNSName + "/pod-1",
			},
		},
		"pPod exists, vPod exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superAssignedPod("pod-2", superDefaultNSName, "12345", "n1", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantAssignedPod("pod-2", "default", "123456", "n1"),
			},
			ExpectedDeletedPPods: []string{
				superDefaultNSName + "/pod-2",
			},
		},
		"pPod exists, vPod exists with different spec": {
			ExistingObjectInSuper: []runtime.Object{
				applyStatusToPod(applySpecToPod(superAssignedPod("pod-3", superDefaultNSName, "12345", "n1", defaultClusterKey), spec2), statusReadyAndRunning),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(applySpecToPod(tenantAssignedPod("pod-3", "default", "12345", "n1"), spec1), statusReadyAndRunning),
			},
			ExpectedUpdatedPPods: []runtime.Object{
				applyStatusToPod(applySpecToPod(superAssignedPod("pod-3", superDefaultNSName, "12345", "n1", defaultClusterKey), spec1), statusReadyAndRunning),
			},
			WaitDWS: true,
		},
		"pPod exists, vPod exists with different status": {
			ExistingObjectInSuper: []runtime.Object{
				applyStatusToPod(superAssignedPod("pod-4", superDefaultNSName, "12345", "n1", defaultClusterKey), statusReadyAndRunning),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-4", "default", "12345", "n1"), statusPending),
				fakeNode("n1"),
			},
			ExpectedUpdatedVPods: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-4", "default", "12345", "n1"), statusReadyAndRunning),
			},
			WaitUWS: true,
		},
		"vPod not scheduled, pPod does not exists": {
			ExistingObjectInSuper: []runtime.Object{
				superSecret("default-token-12345", superDefaultNSName, "12345"),
				superService("kubernetes", superDefaultNSName, "12345", ""),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantPod("pod-5", "default", "12345"),
				tenantServiceAccount("default", "default", "12345"),
			},
			ExpectedCreatedPPods: []string{
				superDefaultNSName + "/pod-5",
			},
			WaitDWS: true,
		},
		"vPod scheduled with DeletionTimestamp, pPod does not exists": {
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(applyDeletionTimestampToPod(tenantAssignedPod("pod-6", "default", "12345", "n1"), time.Now(), 30), statusReadyAndRunning),
			},
			ExpectedDeletedVPods: []string{
				"default/pod-6",
			},
		},
		"vPod scheduled without DeletionTimestamp, pPod does not exists": {
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-7", "default", "12345", "n1"), statusReadyAndRunning),
			},
			ExpectedDeletedVPods: []string{
				"default/pod-7",
			},
		},
		"vPod nodename is not equal to pPod nodename": {
			ExistingObjectInSuper: []runtime.Object{
				applyStatusToPod(superAssignedPod("pod-8", superDefaultNSName, "12345", "n2", defaultClusterKey), statusReadyAndRunning),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-8", "default", "12345", "n1"), statusReadyAndRunning),
			},
			ExpectedDeletedVPods: []string{
				"default/pod-8",
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tenantActions, superActions, err := util.RunPatrol(NewPodController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, nil, tc.WaitDWS, tc.WaitUWS, nil)
			if err != nil {
				t.Errorf("%s: error running patrol: %v", k, err)
				return
			}

			if tc.ExpectedDeletedPPods != nil {
				if len(tc.ExpectedDeletedPPods) != len(superActions) {
					t.Errorf("%s: Expected to delete PPod %#v. Actual actions were: %#v", k, tc.ExpectedDeletedPPods, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedPPods {
					action := superActions[i]
					if !action.Matches("delete", "pods") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pPod %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedDeletedVPods != nil {
				if len(tc.ExpectedDeletedVPods) != len(tenantActions) {
					t.Errorf("%s: Expected to delete VPod %#v. Actual actions were: %#v", k, tc.ExpectedDeletedVPods, tenantActions)
					return
				}
				for i, expectedName := range tc.ExpectedDeletedVPods {
					action := tenantActions[i]
					if !action.Matches("delete", "pods") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					fullName := action.(core.DeleteAction).GetNamespace() + "/" + action.(core.DeleteAction).GetName()
					if fullName != expectedName {
						t.Errorf("%s: Expect to delete pPod %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedCreatedPPods != nil {
				if len(tc.ExpectedCreatedPPods) != len(superActions) {
					t.Errorf("%s: Expected to create PPod %#v. Actual actions were: %#v", k, tc.ExpectedCreatedPPods, superActions)
					return
				}
				for i, expectedName := range tc.ExpectedCreatedPPods {
					action := superActions[i]
					if !action.Matches("create", "pods") {
						t.Errorf("%s: Unexpected action %s", k, action)
						continue
					}
					createdPod := action.(core.CreateAction).GetObject().(*v1.Pod)
					fullName := createdPod.Namespace + "/" + createdPod.Name
					if fullName != expectedName {
						t.Errorf("%s: Expect to create pPod %s, got %s", k, expectedName, fullName)
					}
				}
			}
			if tc.ExpectedUpdatedPPods != nil {
				if len(tc.ExpectedUpdatedPPods) != len(superActions) {
					t.Errorf("%s: Expected to update PPod %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedPPods, superActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedPPods {
					action := superActions[i]
					if !action.Matches("update", "pods") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated pPod is %v, got %v", k, obj, actionObj)
					}
				}
			}
			if tc.ExpectedUpdatedVPods != nil {
				if len(tc.ExpectedUpdatedVPods) != len(tenantActions) {
					t.Errorf("%s: Expected to update VPod %#v. Actual actions were: %#v", k, tc.ExpectedUpdatedVPods, tenantActions)
					return
				}
				for i, obj := range tc.ExpectedUpdatedVPods {
					action := tenantActions[i]
					if !action.Matches("update", "pods") {
						t.Errorf("%s: Unexpected action %s", k, action)
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						t.Errorf("%s: Expected updated vPod is %v, got %v", k, obj, actionObj)
					}
				}
			}
		})
	}
}

func TestVNodeGC(t *testing.T) {
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
	statusReadyAndRunning := &v1.PodStatus{
		Phase: "Running",
		Conditions: []v1.PodCondition{
			{
				Type:   "PodScheduled",
				Status: "True",
			},
		},
	}
	mFunc1 := func(r manager.ResourceSyncer) {
		podController := r.(*controller)
		podController.updateClusterVNodePodMap(defaultClusterKey, "n1", "12345", reconciler.UpdateEvent)
	}
	mFunc2 := func(r manager.ResourceSyncer) {
		podController := r.(*controller)
		podController.updateClusterVNodePodMap(defaultClusterKey, "n1", "12345", reconciler.UpdateEvent)
		// Add "n1" to vNodeGCMap
		podController.updateClusterVNodePodMap(defaultClusterKey, "n1", "12345", reconciler.DeleteEvent)
	}
	mFunc3 := func(r manager.ResourceSyncer) {
		podController := r.(*controller)
		podController.vNodeGCGracePeriod = 0
		podController.updateClusterVNodePodMap(defaultClusterKey, "n1", "12345", reconciler.UpdateEvent)
		// Add "n1" to vNodeGCMap
		podController.updateClusterVNodePodMap(defaultClusterKey, "n1", "12345", reconciler.DeleteEvent)
	}
	mFunc4 := func(r manager.ResourceSyncer) {
		podController := r.(*controller)
		podController.vNodeGCGracePeriod = 0
	}
	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		ExpectedDeletedVNodes  []string
		StateModifyFunc        func(manager.ResourceSyncer)
	}{
		"vNode is bound with a vPod": {
			ExistingObjectInSuper: []runtime.Object{
				superAssignedPod("pod-1", superDefaultNSName, "12345", "n1", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantAssignedPod("pod-1", "default", "12345", "n1"),
				fakeNode("n1"),
			},
			StateModifyFunc:       mFunc1,
			ExpectedDeletedVNodes: []string{},
		},
		"vNode is in gc map with default gc grace period": {
			ExistingObjectInSuper: []runtime.Object{},
			ExistingObjectInTenant: []runtime.Object{
				fakeNode("n1"),
			},
			StateModifyFunc:       mFunc2,
			ExpectedDeletedVNodes: []string{},
		},
		"vNode is in gc map with 0 gc grace period": {
			ExistingObjectInSuper: []runtime.Object{},
			ExistingObjectInTenant: []runtime.Object{
				fakeNode("n1"),
			},
			StateModifyFunc:       mFunc3,
			ExpectedDeletedVNodes: []string{"n1"},
		},
		"orphan vNode with 0 gc grace period": {
			ExistingObjectInSuper: []runtime.Object{},
			ExistingObjectInTenant: []runtime.Object{
				fakeNode("n1"),
			},
			StateModifyFunc:       mFunc4,
			ExpectedDeletedVNodes: []string{"n1"},
		},
		"vNode is INCORRECTLY in gc map with 0 gc grace period": {
			ExistingObjectInSuper: []runtime.Object{
				applyStatusToPod(superAssignedPod("pod-2", superDefaultNSName, "12345", "n1", defaultClusterKey), statusReadyAndRunning),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyStatusToPod(tenantAssignedPod("pod-2", "default", "12345", "n1"), statusReadyAndRunning),
				fakeNode("n1"),
			},
			StateModifyFunc:       mFunc3,
			ExpectedDeletedVNodes: []string{},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			tenantActions, _, err := util.RunPatrol(NewPodController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, nil, false, false, tc.StateModifyFunc)
			if err != nil {
				t.Errorf("%s: error running patrol: %v", k, err)
				return
			}
			if len(tc.ExpectedDeletedVNodes) == 0 && len(tenantActions) != 0 {
				t.Errorf("%s: expect no action but got actions: %v", k, tenantActions)
				return
			}
			for _, expectedName := range tc.ExpectedDeletedVNodes {
				matched := false
				for _, action := range tenantActions {
					if !action.Matches("delete", "nodes") {
						continue
					}
					name := action.(core.DeleteAction).GetName()
					if name != expectedName {
						t.Errorf("%s: Expected %s to be deleted, got %s", k, expectedName, name)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect deleted vNode %s but not found", k, expectedName)
				}
			}
		})
	}
}
