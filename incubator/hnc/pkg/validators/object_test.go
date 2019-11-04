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
		name     string
		oldLabel string
		oldValue string
		newLabel string
		newValue string
		fail     bool
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
