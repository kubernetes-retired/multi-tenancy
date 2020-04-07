package validators

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/metadata"
)

func TestInheritedFromLabel(t *testing.T) {
	f := forest.NewForest()
	o := &Object{Forest: f}
	l := zap.Logger(false)

	tests := []struct {
		name      string
		oldLabel  string
		oldValue  string
		newLabel  string
		newValue  string
		namespace string
		fail      bool
	}{{
		name:     "Regular labels can be changed",
		oldLabel: "oldLabel", oldValue: "foo",
		newLabel: "newLabel", newValue: "bar",
	}, {
		name:     "Label stays the same",
		oldLabel: api.LabelInheritedFrom, oldValue: "foo",
		newLabel: api.LabelInheritedFrom, newValue: "foo",
	}, {
		name:     "Change in label's value",
		oldLabel: api.LabelInheritedFrom, oldValue: "foo",
		newLabel: api.LabelInheritedFrom, newValue: "bar",
		fail: true,
	}, {
		name:     "Label is removed",
		oldLabel: api.LabelInheritedFrom, oldValue: "foo",
		fail: true,
	}, {
		name:     "Label is added",
		newLabel: api.LabelInheritedFrom, newValue: "foo",
		fail: true,
	}, {
		name:     "Objects in excluded namespace is ignored",
		oldLabel: api.LabelInheritedFrom, oldValue: "foo",
		newLabel: api.LabelInheritedFrom, newValue: "bar",
		namespace: "hnc-system",
	},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			oldInst := &unstructured.Unstructured{}
			metadata.SetLabel(oldInst, tc.oldLabel, tc.oldValue)
			inst := &unstructured.Unstructured{}
			inst.SetNamespace(tc.namespace)
			metadata.SetLabel(inst, tc.newLabel, tc.newValue)

			// Test
			got := o.handle(context.Background(), l, inst, oldInst)

			// Report
			reason := got.AdmissionResponse.Result.Reason
			code := got.AdmissionResponse.Result.Code
			t.Logf("Got reason %q, code %d", reason, code)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}

func TestUserChanges(t *testing.T) {

	f := forest.NewForest()
	o := &Object{Forest: f}
	l := zap.Logger(false)

	tests := []struct {
		name    string
		oldInst *unstructured.Unstructured
		inst    *unstructured.Unstructured
		fail    bool
	}{{
		name: "Allow changes to original objects",
		oldInst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]string{
						"testLabel": "1",
					},
				},
			},
		},
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]string{
						"testLabel": "2",
					},
				},
			},
		},
	}, {
		name: "Deny metadata changes to propagated objects",
		fail: true,
		oldInst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						api.LabelInheritedFrom: "foo",
						"testLabel":            "1",
					},
				},
			},
		},
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						api.LabelInheritedFrom: "foo",
						"testLabel":            "2",
					},
				},
			},
		},
	}, {
		name: "Deny spec changes to propagated objects",
		fail: true,
		oldInst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						api.LabelInheritedFrom: "foo",
					},
				},
				"spec": map[string]interface{}{
					"hostname": "hello.com",
				},
			},
		},
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						api.LabelInheritedFrom: "foo",
					},
				},
				"spec": map[string]interface{}{
					"hostname": "world.com",
				},
			},
		},
	}, {
		name: "Allow status changes to propagated objects",
		oldInst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						api.LabelInheritedFrom: "foo",
					},
				},
				"status": map[string]interface{}{
					"message": "hello",
				},
			},
		},
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						api.LabelInheritedFrom: "foo",
					},
				},
				"status": map[string]interface{}{
					"message": "example",
				},
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			// Test
			got := o.handle(context.Background(), l, tc.inst, tc.oldInst)
			// Report
			reason := got.AdmissionResponse.Result.Reason
			code := got.AdmissionResponse.Result.Code
			t.Logf("Got reason %q, code %d", reason, code)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}
