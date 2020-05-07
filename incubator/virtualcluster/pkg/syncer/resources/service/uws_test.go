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

package service

import (
	"encoding/json"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	core "k8s.io/client-go/testing"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/test"
)

func applyLoadBalancerToService(svc *v1.Service, ip string) *v1.Service {
	svc.Spec.Type = v1.ServiceTypeLoadBalancer
	svc.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{
		{
			IP: ip,
		},
	}
	return svc
}

func TestUWService(t *testing.T) {
	testTenant := &v1alpha1.Virtualcluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "tenant-1",
			UID:       "7374a172-c35d-45b1-9c8e-bf5c5b614937",
		},
		Status: v1alpha1.VirtualclusterStatus{
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
		"pService not found": {
			ExistingObjectInTenant: []runtime.Object{
				tenantService("svc", "default", "12345"),
			},
			EnqueuedKey:         superDefaultNSName + "/svc",
			ExpectedNoOperation: true,
		},
		"pService not created by syncer": {
			ExistingObjectInSuper: []runtime.Object{
				tenantService("kubernetes", superDefaultNSName, "12345"),
			},
			EnqueuedKey:         superDefaultNSName + "/kubernetes",
			ExpectedNoOperation: true,
		},
		"pService exists but vService does not exist": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc", superDefaultNSName, "12345", defaultClusterKey),
			},
			EnqueuedKey:   superDefaultNSName + "/svc",
			ExpectedError: "",
		},
		"pService exists, vService exists with different uid": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantService("svc", "default", "12345"),
			},
			EnqueuedKey:   superDefaultNSName + "/svc",
			ExpectedError: "delegated UID is different",
		},
		"pService exists, vService exists with no diff": {
			ExistingObjectInSuper: []runtime.Object{
				superService("svc", superDefaultNSName, "123456", defaultClusterKey),
			},
			ExistingObjectInTenant: []runtime.Object{
				tenantService("svc", "default", "12345"),
			},
			EnqueuedKey:         superDefaultNSName + "/svc",
			ExpectedNoOperation: true,
		},
		"pService exists, vService exists with different status": {
			ExistingObjectInSuper: []runtime.Object{
				applyLoadBalancerToService(superService("svc", superDefaultNSName, "12345", defaultClusterKey), "1.1.1.1"),
			},
			ExistingObjectInTenant: []runtime.Object{
				applyLoadBalancerToService(tenantService("svc", "default", "12345"), "1.1.1.2"),
			},
			EnqueuedKey: superDefaultNSName + "/svc",
			ExpectedUpdatedObject: []runtime.Object{
				applyLoadBalancerToService(tenantService("svc", "default", "12345"), "1.1.1.1"),
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			actions, reconcileErr, err := util.RunUpwardSync(NewServiceController, testTenant, tc.ExistingObjectInSuper, tc.ExistingObjectInTenant, tc.EnqueuedKey)
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
					if !action.Matches("update", "services") {
						continue
					}
					actionObj := action.(core.UpdateAction).GetObject()
					if !equality.Semantic.DeepEqual(obj, actionObj) {
						exp, _ := json.Marshal(obj)
						got, _ := json.Marshal(actionObj)
						t.Errorf("%s: Expected updated Service is %v, got %v", k, string(exp), string(got))
					}
					matched = true
					break
				}
				if !matched {
					t.Errorf("%s: Expect updated Service %+v but not found", k, obj)
				}
			}
		})
	}
}
