package validators

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"
	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/foresttest"
)

func TestCreateSubnamespaces(t *testing.T) {
	// Creat namespace "a" as the root with one subnamespace "b" and one full child
	// namespace "c".
	f := foresttest.Create("-Aa")
	h := &Anchor{Forest: f}

	tests := []struct {
		name string
		pnm  string
		cnm  string
		fail bool
	}{
		{name: "with a non-existing name", pnm: "a", cnm: "brumpf"},
		{name: "in excluded ns", pnm: "kube-system", cnm: "brumpf", fail: true},
		{name: "with an existing ns name (the ns is not a subnamespace of it)", pnm: "c", cnm: "b", fail: true},
		{name: "for existing non-subns child", pnm: "a", cnm: "c", fail: true},
		{name: "for existing subns", pnm: "a", cnm: "b"},
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
				op:     v1beta1.Create,
			}

			// Test
			got := h.handle(req)

			// Report
			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}

func TestAllowCascadingDeleteSubnamespaces(t *testing.T) {
	// Create a chain of namespaces from "a" to "e", with "a" as the root. Among them,
	// "b", "d" and "e" are subnamespaces. This is set up in a long chain to test that
	// subnamespaces will look all the way up to get the 'allowCascadingDelete` value
	// and won't stop looking when the first full namespace ancestor is met.
	f := foresttest.Create("-AbCD")
	h := &Anchor{Forest: f}

	tests := []struct {
		name string
		acd  string
		pnm  string
		cnm  string
		fail bool
	}{
		{name: "set in parent", acd: "c", pnm: "c", cnm: "d"},
		{name: "set in non-leaf", acd: "d", pnm: "c", cnm: "d"},
		{name: "set in ancestor that is not the first full namespace", acd: "a", pnm: "c", cnm: "d"},
		{name: "unset in leaf", pnm: "d", cnm: "e"},
		{name: "unset in non-leaf", pnm: "c", cnm: "d", fail: true},
		{name: "unset in non-leaf but bad anchor", pnm: "b", cnm: "d"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.acd != "" {
				f.Get(tc.acd).UpdateAllowCascadingDelete(true)
				defer f.Get(tc.acd).UpdateAllowCascadingDelete(false)
			}

			// Setup
			g := NewGomegaWithT(t)
			anchor := &api.SubnamespaceAnchor{}
			anchor.ObjectMeta.Namespace = tc.pnm
			anchor.ObjectMeta.Name = tc.cnm
			req := &anchorRequest{
				anchor: anchor,
				op:     v1beta1.Delete,
			}

			// Test
			got := h.handle(req)

			// Report
			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}
