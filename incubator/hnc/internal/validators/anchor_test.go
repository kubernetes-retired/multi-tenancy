package validators

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/foresttest"
)

func TestSubnamespaces(t *testing.T) {
	// Two roots `a` and `b`, with `c` a subnamespace of `a`
	f := foresttest.Create("--A")
	h := &Anchor{Forest: f}
	f.Get("c").UpdateAllowCascadingDelete(true)

	tests := []struct {
		name string
		op   v1beta1.Operation
		pnm  string
		cnm  string
		fail bool
	}{
		{name: "ok-create", op: v1beta1.Create, pnm: "a", cnm: "brumpf"},
		{name: "ok-delete", op: v1beta1.Delete, pnm: "a", cnm: "c"},
		{name: "create anchor in excluded ns", op: v1beta1.Create, pnm: "kube-system", cnm: "brumpf", fail: true},
		{name: "create anchor with existing ns name", op: v1beta1.Create, pnm: "a", cnm: "b", fail: true},
		{name: "delete anchor when allowCascadingDelete is false", op: v1beta1.Delete, pnm: "a", cnm: "b", fail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			anchor := &api.SubnamespaceAnchor{}
			anchor.ObjectMeta.Namespace = tc.pnm
			anchor.ObjectMeta.Name = tc.cnm
			req := &anchorRequest{
				anchor: anchor,
				op:     tc.op,
			}

			// Test
			got := h.handle(req)

			// Report
			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}
