package validators

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/foresttest"
)

func TestDeleteSubNamespace(t *testing.T) {

	// We will put an anchor a in the namespace c if it exists.
	tests := []struct {
		name string
		// We only use the forest here to create full namespace hierarchy.
		forest string
		// The subnamespaces are created by manually adding the annotation.
		subnamespaceOf string
		fail           bool
	}{
		// There's no test case for when parent and annotation don't match, since the
		// reconciler always sets the parent to match the subnamespaceOf annotation.
		// - a (subnamespaceOf b)
		{name: "when parent is missing", forest: "-", subnamespaceOf: "b"},
		// - a (subnamespaceOf b) -> b (no anchor)
		{name: "when anchor is missing", forest: "b-", subnamespaceOf: "b"},
		// - a (subnamespaceOf c) -> c (has anchor a), b
		{name: "when annotation and anchor match", forest: "c--", subnamespaceOf: "c", fail: true},
		// - a (subnamespaceOf b) -> b, c(has anchor a)
		{name: "when annotation and anchor don't match", forest: "b--", subnamespaceOf: "b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)

			// Create a namespace instance a and add the subnamespaceOf annotation.
			sub := &corev1.Namespace{}
			sub.Name = "a"
			setSubAnnotation(sub, tc.subnamespaceOf)

			req := &nsRequest{
				ns: sub,
				op: v1beta1.Delete,
			}

			// Construct the forest
			f := foresttest.Create(tc.forest)
			// Add anchor "a" to namespace "c" if it exists. This is to test the cases
			// when the subnamespaceOf annotation in "a" matches/dismatches "c".
			if f.Get("c").Exists() {
				f.Get("c").SetAnchors([]string{"a"})
			}
			vns := &Namespace{Forest: f}

			// Test
			got := vns.handle(req)

			// Report
			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
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

func TestCreateNamespace(t *testing.T) {
	// nm is the name of the namespace to be created, which already exists in external hierarchy.
	nm := "exhier"

	// Create a single external namespace "a" with "exhier" in the external hierarchy.
	f := foresttest.Create("-")
	vns := &Namespace{Forest: f}
	a := f.Get("a")
	a.ExternalTreeLabels = map[string]int{
		nm:       1,
		a.Name(): 0,
	}

	// Requested namespace uses "exhier" as name.
	ns := &corev1.Namespace{}
	ns.Name = nm

	t.Run("Create namespace with an already existing name in external hierarchy", func(t *testing.T) {
		g := NewGomegaWithT(t)
		req := &nsRequest{
			ns: ns,
			op: v1beta1.Create,
		}

		// Test
		got := vns.handle(req)

		// Report
		logResult(t, got.AdmissionResponse.Result)
		g.Expect(got.AdmissionResponse.Allowed).Should(BeFalse())
	})
}

func TestUpdateNamespaceManagedBy(t *testing.T) {
	f := foresttest.Create("-a-c") // a <- b; c <- d
	vns := &Namespace{Forest: f}

	aInst := &corev1.Namespace{}
	aInst.Name = "a"
	bInst := &corev1.Namespace{}
	bInst.Name = "b"

	// Add 'hnc.x-k8s.io/managedBy: other' annotation on c.
	cInst := &corev1.Namespace{}
	cInst.Name = "c"
	cInst.SetAnnotations(map[string]string{api.AnnotationManagedBy: "other"})

	// ** Please note this will make d in an *illegal* state. **
	// Add 'hnc.x-k8s.io/managedBy: other' annotation on d.
	dInst := &corev1.Namespace{}
	dInst.Name = "d"
	dInst.SetAnnotations(map[string]string{api.AnnotationManagedBy: "other"})

	// These cases test converting namespaces between internal and external, described
	// in the table at https://bit.ly/hnc-external-hierarchy#heading=h.z9mkbslfq41g
	// with other cases covered in the hierarchy_test.go.
	tests := []struct {
		name      string
		nsInst    *corev1.Namespace
		managedBy string
		fail      bool
	}{
		{name: "ok: default (no annotation)", nsInst: aInst, managedBy: ""},
		{name: "ok: explicitly managed by HNC", nsInst: aInst, managedBy: "hnc.x-k8s.io"},
		{name: "ok: convert a root internal namespace to external", nsInst: aInst, managedBy: "other"},
		{name: "not ok: convert a non-root internal namespace to external", nsInst: bInst, managedBy: "other", fail: true},
		{name: "ok: convert an external namespace to internal by changing annotation value", nsInst: cInst, managedBy: "hnc.x-k8s.io"},
		{name: "ok: convert an external namespace to internal by removing annotation", nsInst: cInst, managedBy: ""},
		{name: "ok: resolve illegal state by changing annotation value", nsInst: dInst, managedBy: "hnc.x-k8s.io"},
		{name: "ok: resolve illegal state by removing annotation", nsInst: dInst, managedBy: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			tnsInst := tc.nsInst
			if tc.managedBy == "" {
				tnsInst.SetAnnotations(map[string]string{})
			} else {
				tnsInst.SetAnnotations(map[string]string{api.AnnotationManagedBy: tc.managedBy})
			}

			req := &nsRequest{
				ns: tc.nsInst,
				op: v1beta1.Update,
			}

			// Test
			got := vns.handle(req)

			// Report
			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}

// setSubAnnotations sets subnamespaceOf annotation with a parent name on the
// namespace. If the parent name is empty, it removes the annotation.
func setSubAnnotation(ns *corev1.Namespace, pnm string) {
	a := make(map[string]string)
	if pnm != "" {
		a[api.SubnamespaceOf] = pnm
	}
	ns.SetAnnotations(a)
}
