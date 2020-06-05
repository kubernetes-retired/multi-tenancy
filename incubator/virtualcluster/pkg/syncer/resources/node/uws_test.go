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

package node

import (
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func makeNode(name string) *v1.Node {
	return &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.LabelOSStable:   "linux",
				v1.LabelArchStable: "amd64",
				v1.LabelHostname:   "n1",
			},
		},
	}
}

func TestUWNode(t *testing.T) {
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

	mFunc1 := func(r manager.ResourceSyncer) {
		nodeController := r.(*controller)
		nodeController.nodeNameToCluster = map[string]map[string]struct{}{
			"n1": {
				defaultClusterKey: struct{}{},
			},
		}
	}

	testcases := map[string]struct {
		ExistingObjectInSuper  []runtime.Object
		ExistingObjectInTenant []runtime.Object
		EnqueuedKey            string
		ExpectedUpdatedObject  []string
		ExpectedNoOperation    bool
		ExpectedError          string
		StateModifyFunc        func(manager.ResourceSyncer)
	}{
		"pNode not found": {
			EnqueuedKey:         "n1",
			ExpectedNoOperation: true,
		},
		"pNode exists but does not belong to any tenant": {
			ExistingObjectInSuper: []runtime.Object{
				makeNode("n1"),
			},
			EnqueuedKey:         "n1",
			ExpectedNoOperation: true,
		},
		"pNode exists but vNode does not exist": {
			ExistingObjectInSuper: []runtime.Object{
				makeNode("n1"),
			},
			EnqueuedKey:         "n1",
			StateModifyFunc:     mFunc1,
			ExpectedNoOperation: true,
		},
		"pNode vNode in normal state": {
			ExistingObjectInSuper: []runtime.Object{
				makeNode("n1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				makeNode("n1"),
			},
			EnqueuedKey:     "n1",
			StateModifyFunc: mFunc1,
			ExpectedUpdatedObject: []string{
				"n1",
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewNodeController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, tc.StateModifyFunc)
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

			for _, expectedName := range tc.ExpectedUpdatedObject {
				matched := false
				for _, action := range actions {
					if !action.Matches("patch", "nodes") {
						continue
					}
					name := action.(core.PatchAction).GetName()
					if name != expectedName {
						t.Errorf("%s: Expected %s to be patched, got %s", k, expectedName, name)
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect patch Node %+v but not found", k, expectedName)
				}
			}
		})
	}
}
