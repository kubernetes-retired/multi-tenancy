package validators

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/foresttest"

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
	l := zap.Logger(false)

	tests := []struct {
		name       string
		oldInst    *unstructured.Unstructured
		inst       *unstructured.Unstructured
		fail       bool
		isDeleting bool
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
		name:       "Deny deletions of propagated objects",
		fail:       true,
		isDeleting: true,
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
	}, {
		name: "Deny creation of object with invalid select annotation",
		fail: true,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationSelector: "$foo",
					},
				},
			},
		},
	}, {
		name: "Allow creation of object with valid select annotation",
		fail: false,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationSelector: "foo",
					},
				},
			},
		},
	}, {
		name: "Deny creation of object with invalid treeSelect annotation",
		fail: true,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationTreeSelector: "foo, $bar",
					},
				},
			},
		},
	}, {
		name: "Allow creation of object with valid treeSelect annotation",
		fail: false,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationTreeSelector: "foo, !bar",
					},
				},
			},
		},
	}, {
		name: "Deny creation of object with invalid noneSelect annotation",
		fail: true,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationNoneSelector: "foo",
					},
				},
			},
		},
	}, {
		name: "Allow creation of object with valid noneSelect annotation",
		fail: false,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationNoneSelector: "true",
					},
				},
			},
		},
	}, {
		name: "Deny creation of object with invalid selector and valid treeSelect annotation",
		fail: true,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationSelector:     "$foo",
						api.AnnotationTreeSelector: "!bar",
					},
				},
			},
		},
	}, {
		name: "Deny creation of object with valid selector and invalid treeSelect annotation",
		fail: true,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationSelector:     "foo",
						api.AnnotationTreeSelector: "$bar",
					},
				},
			},
		},
	}, {
		name: "Deny creation of object with both selector and treeSelect annotation",
		fail: true,
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationSelector:     "foo",
						api.AnnotationTreeSelector: "!bar",
					},
				},
			},
		},
	}, {
		name: "Allow object with multiple selectors if it's not a selector change",
		fail: false,
		oldInst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationSelector:     "foo",
						api.AnnotationTreeSelector: "!bar",
					},
				},
			},
		},
		inst: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"annotations": map[string]interface{}{
						api.AnnotationSelector:     "foo",
						api.AnnotationTreeSelector: "!bar",
					},
					"status": map[string]interface{}{
						"message": "hello",
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
			c := fakeClient{isDeleting: tc.isDeleting}
			o := &Object{Forest: f, client: c}
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

// fakeClient implements a fake client (https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client#Client).
type fakeClient struct {
	isDeleting bool
}

// Get returns error if the namespace is being deleted
func (c fakeClient) Get(_ context.Context, key client.ObjectKey, obj runtime.Object) error {
	if c.isDeleting {
		return &errors.StatusError{
			ErrStatus: metav1.Status{
				Status:  metav1.StatusFailure,
				Reason:  metav1.StatusReasonNotFound,
				Message: fmt.Sprintf("%s not found", key),
			}}
	}
	return nil
}

// Create fake implementation of client.Client
func (c fakeClient) Create(_ context.Context, _ runtime.Object, _ ...client.CreateOption) error {
	return nil
}

// Update fake implementation of client.Client
func (c fakeClient) Update(_ context.Context, _ runtime.Object, _ ...client.UpdateOption) error {
	return nil
}

// Delete fake implementation of client.Client
func (c fakeClient) Delete(ctx context.Context, _ runtime.Object, _ ...client.DeleteOption) error {
	return nil
}

// DeleteAllOf fake implementation of client.Client
func (c fakeClient) DeleteAllOf(ctx context.Context, _ runtime.Object, _ ...client.DeleteAllOfOption) error {
	return nil
}

// Patch fake implementation of client.Client
func (c fakeClient) Patch(ctx context.Context, _ runtime.Object, _ client.Patch, _ ...client.PatchOption) error {
	return nil
}

// List fake implementation of client.Client
func (c fakeClient) List(ctx context.Context, _ runtime.Object, _ ...client.ListOption) error {
	return nil
}

// Status fake implementation of client.StatusClient
func (c fakeClient) Status() client.StatusWriter {
	return nil
}

func TestCreatingConflictSource(t *testing.T) {
	tests := []struct {
		name              string
		forest            string
		conflictInstName  string
		conflictNamespace string
		newInstName       string
		newInstNamespace  string
		newInstAnnotation map[string]string
		fail              bool
	}{{
		name:              "Deny creation of source objects with conflict in child",
		forest:            "-a",
		conflictInstName:  "secret-b",
		conflictNamespace: "b",
		newInstName:       "secret-b",
		newInstNamespace:  "a",
		fail:              true,
	}, {
		name:              "Deny creation of source objects with conflict in grandchild",
		forest:            "-ab",
		conflictInstName:  "secret-c",
		conflictNamespace: "c",
		newInstName:       "secret-c",
		newInstNamespace:  "a",
		fail:              true,
	}, {
		name:             "Allow creation of source objects with no conflict",
		newInstName:      "secret-a",
		newInstNamespace: "a",
	}, {
		name:              "Allow creation of source objects with treeSelector not matching the conflicting child",
		forest:            "-aa",
		conflictInstName:  "secret-b",
		conflictNamespace: "b",
		newInstName:       "secret-b",
		newInstNamespace:  "a",
		newInstAnnotation: map[string]string{api.AnnotationTreeSelector: "c"},
		fail:              false,
	}, {
		name:              "Allow creation of source objects with selector not matching the conflicting child",
		forest:            "-aa",
		conflictInstName:  "secret-b",
		conflictNamespace: "b",
		newInstName:       "secret-b",
		newInstNamespace:  "a",
		newInstAnnotation: map[string]string{api.AnnotationSelector: "c" + api.LabelTreeDepthSuffix},
		fail:              false,
	}, {
		name:              "Allow creation of source objects with noneSelector set",
		forest:            "-aa",
		conflictInstName:  "secret-b",
		conflictNamespace: "b",
		newInstName:       "secret-b",
		newInstNamespace:  "a",
		newInstAnnotation: map[string]string{api.AnnotationNoneSelector: "true"},
		fail:              false,
	}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			f := foresttest.Create(tc.forest)
			createSecret(tc.conflictInstName, tc.conflictNamespace, f)
			o := &Object{Forest: f}
			l := zap.Logger(false)
			op := admissionv1beta1.Create
			inst := &unstructured.Unstructured{}
			inst.SetName(tc.newInstName)
			inst.SetNamespace(tc.newInstNamespace)
			inst.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})
			inst.SetAnnotations(tc.newInstAnnotation)
			// Test
			got := o.handle(context.Background(), l, op, inst, &unstructured.Unstructured{})
			// Report
			code := got.AdmissionResponse.Result.Code
			reason := got.AdmissionResponse.Result.Reason
			msg := got.AdmissionResponse.Result.Message
			t.Logf("Got code %d, reason %q, message %q", code, reason, msg)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}

func createSecret(nm, nsn string, f *forest.Forest) {
	if nm == "" || nsn == "" {
		return
	}
	inst := &unstructured.Unstructured{}
	inst.SetName(nm)
	inst.SetNamespace(nsn)
	inst.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"})
	f.Get(nsn).SetSourceObject(inst)
}
