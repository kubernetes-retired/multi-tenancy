package validators

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

func TestHNS(t *testing.T) {
	f := forest.NewForest()

	// Create two namespaces foo and bar.
	createNS(f, "foo", nil)
	createNS(f, "bar", nil)

	// Create subnamespace baz for foo with AllowCascadingDelete set to true.
	baz := createOwnedNamespace(f, "foo", "baz")
	baz.UpdateAllowCascadingDelete(true)

	h := &Anchor{Forest: f}

	tests := []struct {
		name string
		op   v1beta1.Operation
		pnm  string
		cnm  string
		fail bool
	}{
		{name: "ok-create", op: v1beta1.Create, pnm: "foo", cnm: "brumpf"},
		{name: "ok-delete", op: v1beta1.Delete, pnm: "foo", cnm: "baz"},
		{name: "create anchor in excluded ns", op: v1beta1.Create, pnm: "kube-system", cnm: "brumpf", fail: true},
		{name: "create anchor with existing ns name", op: v1beta1.Create, pnm: "foo", cnm: "bar", fail: true},
		{name: "delete anchor when allowCascadingDelete is false", op: v1beta1.Delete, pnm: "foo", cnm: "bar", fail: true},
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

func createOwnedNamespace(f *forest.Forest, pnm, cnm string) *forest.Namespace {
	pns := f.Get(pnm)
	cns := createNS(f, cnm, nil)
	cns.SetParent(pns)
	cns.IsSub = true
	return cns
}
