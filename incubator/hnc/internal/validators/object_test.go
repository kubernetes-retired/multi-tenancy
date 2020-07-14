package validators

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/metadata"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/reconcilers"
)

// TestEarlyExit tests requests that, without an early exit, would *definitely* be denied because
// they don't include any actual objects to validate. The only way these tests can pass is if we
// never actually try to decode the object - that is, we do a very early exit because HNC isn't
// supposed to look at these objects in the first place.
func TestType(t *testing.T) {
	or := &reconcilers.ObjectReconciler{
		GVK:  schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
		Mode: api.Propagate,
	}
	f := forest.NewForest()
	f.AddTypeSyncer(or)
	l := zap.Logger(false)
	o := &Object{Forest: f, Log: l}

	tests := []struct {
		name    string
		version string
		kind    string
		ns      string
		deny    bool
	}{{
		name:    "Deny request with GroupVersionKind in the propagate mode",
		version: "v1",
		kind:    "Secret",
		deny:    true,
	}, {
		name:    "Deny request with GroupKind in the propagate mode even if the Version is different",
		version: "v1beta1",
		kind:    "Secret",
		deny:    true,
	}, {
		name: "Always allow request with GroupKind not in propagate mode",
		kind: "Configmap",
	}, {
		name:    "Allow request in excluded namespace",
		version: "v1",
		kind:    "Secret",
		ns:      "kube-system",
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			if tc.ns == "" {
				tc.ns = "default"
			}
			req := admission.Request{AdmissionRequest: admissionv1beta1.AdmissionRequest{
				Name:      "foo",
				Namespace: tc.ns,
				Kind:      metav1.GroupVersionKind{Version: tc.version, Kind: tc.kind},
			}}
			// Test
			got := o.Handle(context.Background(), req)
			// Report
			code := got.AdmissionResponse.Result.Code
			reason := got.AdmissionResponse.Result.Reason
			msg := got.AdmissionResponse.Result.Message
			t.Logf("Got code %d, reason %q, message %q", code, reason, msg)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.deny))
		})
	}
}

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
	}}

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
			got := o.handle(context.Background(), l, admissionv1beta1.Update, inst, oldInst)

			// Report
			code := got.AdmissionResponse.Result.Code
			reason := got.AdmissionResponse.Result.Reason
			msg := got.AdmissionResponse.Result.Message
			t.Logf("Got code %d, reason %q, message %q", code, reason, msg)
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
	}, {
		name: "Allow deletions of source objects",
		oldInst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
			},
		},
	}, {
		name: "Deny deletions of propagated objects",
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
			},
		},
	}, {
		name: "Allow creation of source objects",
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
			},
		},
	}, {
		name: "Deny creation of object with inheritedFrom label",
		fail: true,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"labels": map[string]interface{}{
						api.LabelInheritedFrom: "foo",
					},
				},
			},
		},
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			op := admissionv1beta1.Update
			if tc.inst == nil {
				op = admissionv1beta1.Delete
				tc.inst = &unstructured.Unstructured{}
			} else if tc.oldInst == nil {
				op = admissionv1beta1.Create
				tc.oldInst = &unstructured.Unstructured{}
			}
			// Test
			got := o.handle(context.Background(), l, op, tc.inst, tc.oldInst)
			// Report
			code := got.AdmissionResponse.Result.Code
			reason := got.AdmissionResponse.Result.Reason
			msg := got.AdmissionResponse.Result.Message
			t.Logf("Got code %d, reason %q, message %q", code, reason, msg)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}
