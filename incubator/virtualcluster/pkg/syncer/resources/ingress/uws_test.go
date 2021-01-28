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

package ingress

import (
	"encoding/json"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	utilscheme "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/scheme"
	util "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func applyLoadBalancerToIngress(ing *v1beta1.Ingress, ip string) *v1beta1.Ingress {
	ing.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{
		{
			IP: ip,
		},
	}
	return ing
}

func TestUWIngress(t *testing.T) {
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
		ExpectedUpdatedObject  []runtime.Object
		ExpectedNoOperation    bool
		ExpectedError          string
	}{
		"pIngress not found": {
			ExistingObjectInTenant: []runtime.Object{
				tenantIngress("ing", "default", "12345"),
			},
			EnqueuedKey:         superDefaultNSName + "/ing",
			ExpectedNoOperation: true,
		},
		"pIngress not created by syncer": {
			ExistingObjectInSuper: []runtime.Object{
				tenantIngress("kubernetes", superDefaultNSName, "12345"),
			},
			EnqueuedKey:         superDefaultNSName + "/kubernetes",
			ExpectedNoOperation: true,
		},
		"pIngress exists but vIngress does not exist": {
			ExistingObjectInSuper: []runtime.Object{
				superIngress("ing", superDefaultNSName, "12345", defaultClusterKey),
			},
			EnqueuedKey:   superDefaultNSName + "/ing",
			ExpectedError: "",
		},
		"pIngress exists, vIngress exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superIngress("ing", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantIngress("ing", "default", "12345"),
			},
			EnqueuedKey:   superDefaultNSName + "/ing",
			ExpectedError: "delegated UID is different",
		},
		"pIngress exists, vIngress exists with no diff": {
			ExistingObjectInSuper: []runtime.Object{
				superIngress("ing", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantIngress("ing", "default", "12345"),
			},
			EnqueuedKey:         superDefaultNSName + "/ing",
			ExpectedNoOperation: true,
		},
	}

	utilscheme.Scheme.AddKnownTypePair(&v1beta1.Ingress{}, &v1beta1.IngressList{})
	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewIngressController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey, nil)
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

			for _, obj := range tc.ExpectedUpdatedObject {
				matched := false
				for _, action := range actions {
					if !action.Matches("update", "ingresses") {
						continue
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						exp, _ := json.Marshal(obj)
						got, _ := json.Marshal(actionObj)
						t.Errorf("%s: Expected updated Ingress is %v, got %v", k, string(exp), string(got))
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated Ingress %+v but not found", k, obj)
				}
			}
		})
	}
}
