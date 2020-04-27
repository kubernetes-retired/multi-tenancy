package validators

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

func TestDeleteOwnedNamespace(t *testing.T) {
	// Create a namespace with owner annotation.
	owned := &corev1.Namespace{}
	owned.Name = "owned"
	setOwnerAnnotation(owned)

	vns := &Namespace{}

	t.Run("Delete namespace with owner annotation", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := &nsRequest{
			ns: owned,
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
	owner := &corev1.Namespace{}
	owner.Name = "owner"
	ons := createNS(f, "owner", nil)

	t.Run("Delete a namespace with owned namespaces", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := &nsRequest{
			ns: owner,
			op: v1beta1.Delete,
		}

		// Add two owned namespaces and leave allowCascadingDelete unset.
		cns1 := createOwnedNamespace(f, "owner", "owned1")
		cns2 := createOwnedNamespace(f, "owner", "owned2")
		// Test
		got := vns.handle(req)
		// Report - Shouldn't allow deleting the owner namespace.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())

		// Set allowCascadingDelete on one child.
		cns1.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Still shouldn't allow deleting the owner namespace.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())

		// Set allowCascadingDelete on the other child too.
		cns2.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Should allow deleting the owner namespace since both owned namespaces allow cascading deletion.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeTrue())

		// Unset allowCascadingDelete on one child but set allowCascadingDelete on the owner itself.
		cns2.UpdateAllowCascadingDelete(false)
		ons.UpdateAllowCascadingDelete(true)
		// Test
		got = vns.handle(req)
		// Report - Should allow deleting the owner namespace with allowCascadingDelete set on it.
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeTrue())

	})

}

func setOwnerAnnotation(ns *corev1.Namespace) {
	a := make(map[string]string)
	a[api.AnnotationOwner] = "someOwner"
	ns.SetAnnotations(a)
}
