package validators

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/foresttest"
)

func TestDeleteSubNamespace(t *testing.T) {
	// Create a namespace with owner annotation.
	sub := &corev1.Namespace{}
	sub.Name = "sub"
	setSubAnnotation(sub)

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
	f := foresttest.Create("-AA")
	vns := &Namespace{Forest: f}
	a := f.Get("a")
	aInst := &corev1.Namespace{}
	aInst.Name = "a"
	b := f.Get("b")
	c := f.Get("c")

	t.Run("Delete a namespace with subnamespaces", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := &nsRequest{
			ns: aInst,
			op: v1beta1.Delete,
		}

		// Test
		got := vns.handle(req)
		// Report - Shouldn't allow deleting the parent namespace.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())

		// Set allowCascadingDelete on one child.
		b.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Still shouldn't allow deleting the parent namespace.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())

		// Set allowCascadingDelete on the other child too.
		c.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Should allow deleting the parent namespace since both subnamespaces allow cascading deletion.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeTrue())

		// Unset allowCascadingDelete on one child but set allowCascadingDelete on the parent itself.
		c.UpdateAllowCascadingDelete(false)
		a.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Should allow deleting the parent namespace with allowCascadingDelete set on it.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeTrue())

	})

}

func setSubAnnotation(ns *corev1.Namespace) {
	a := make(map[string]string)
	a[api.SubnamespaceOf] = "someParent"
	ns.SetAnnotations(a)
}
