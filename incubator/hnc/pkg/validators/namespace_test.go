package validators

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

func TestDeleteSubNamespace(t *testing.T) {
	// Create a namespace with owner annotation.
	sub := &corev1.Namespace{}
	sub.Name = "sub"
	setSubnamespaceOf(sub)

	vns := &Namespace{}

	t.Run("Delete namespace with owner annotation", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := &nsRequest{
			ns: sub,
			op: v1beta1.Delete,
		}

		// Test
		got := vns.handle(req)

		// Report
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())
	})
}

func TestDeleteOwnerNamespace(t *testing.T) {
	f := forest.NewForest()
	vns := &Namespace{Forest: f}

	// Create a namespace
	parent := &corev1.Namespace{}
	parent.Name = "parent"
	ons := createNS(f, "parent", nil)

	t.Run("Delete a namespace with subnamespaces", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := &nsRequest{
			ns: parent,
			op: v1beta1.Delete,
		}

		// Add two subnamespaces and leave allowCascadingDelete unset.
		sub1 := createOwnedNamespace(f, "parent", "sub1")
		sub2 := createOwnedNamespace(f, "parent", "sub2")
		// Test
		got := vns.handle(req)
		// Report - Shouldn't allow deleting the parent namespace.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())

		// Set allowCascadingDelete on one child.
		sub1.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Still shouldn't allow deleting the parent namespace.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())

		// Set allowCascadingDelete on the other child too.
		sub2.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Should allow deleting the parent namespace since both subnamespaces allow cascading deletion.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeTrue())

		// Unset allowCascadingDelete on one child but set allowCascadingDelete on the parent itself.
		sub2.UpdateAllowCascadingDelete(false)
		ons.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Should allow deleting the parent namespace with allowCascadingDelete set on it.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeTrue())

	})

}

func setSubnamespaceOf(ns *corev1.Namespace) {
	a := make(map[string]string)
	a[api.SubnamespaceOf] = "someParent"
	ns.SetAnnotations(a)
}
