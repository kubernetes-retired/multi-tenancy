package validators

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

func TestHierarchy(t *testing.T) {
	f := forest.NewForest()
	foo := f.Get("foo")
	bar := f.Get("bar")
	baz := f.Get("baz")
	foo.SetExists()
	bar.SetExists()
	baz.SetExists()
	bar.SetParent(foo)
	h := &Hierarchy{Forest: f}
	l := zap.Logger(false)

	tests := []struct {
		name string
		nnm  string
		pnm  string
		fail bool
	}{
		{name: "ok", nnm: "foo", pnm: "baz"},
		{name: "missing parent", nnm: "foo", pnm: "brumpf"},
		{name: "self-cycle", nnm: "foo", pnm: "foo", fail: true},
		{name: "other cycle", nnm: "foo", pnm: "bar", fail: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			hier := &api.HierarchyConfiguration{Spec: api.HierarchyConfigurationSpec{Parent: tc.pnm}}
			hier.ObjectMeta.Name = api.Singleton
			hier.ObjectMeta.Namespace = tc.nnm

			// Test
			got := h.handle(context.Background(), l, hier)

			// Report
			reason := got.AdmissionResponse.Result.Reason
			code := got.AdmissionResponse.Result.Code
			t.Logf("Got reason %q, code %d", reason, code)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}
