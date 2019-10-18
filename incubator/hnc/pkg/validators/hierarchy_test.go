package validators

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	authn "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

func TestStructure(t *testing.T) {
	f := forest.NewForest()
	foo := createNS(f, "foo", nil)
	bar := createNS(f, "bar", nil)
	createNS(f, "baz", nil)
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
			hc := &api.HierarchyConfiguration{Spec: api.HierarchyConfigurationSpec{Parent: tc.pnm}}
			hc.ObjectMeta.Name = api.Singleton
			hc.ObjectMeta.Namespace = tc.nnm
			req := &request{hc: hc}

			// Test
			got := h.handle(context.Background(), l, req)

			// Report
			logResult(t, got.AdmissionResponse.Result)
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}

func TestAuthz(t *testing.T) {
	tests := []struct {
		name    string
		authz   fakeAuthz
		from    string
		to      string
		fail    bool
		unexist []string
	}{
		{name: "no permission in tree", from: "c", to: "g", fail: true},
		{name: "root permission in tree", from: "c", to: "g", authz: fakeAuthz{"a"}},
		{name: "cur parent only in tree", from: "c", to: "g", authz: fakeAuthz{"c"}, fail: true},
		{name: "dst only in tree", from: "c", to: "g", authz: fakeAuthz{"g"}, fail: true},
		{name: "dst only across trees", from: "c", to: "h", authz: fakeAuthz{"h"}, fail: true},
		{name: "cur root only across trees", from: "c", to: "h", authz: fakeAuthz{"a"}, fail: true},
		{name: "dst and cur parent across trees", from: "c", to: "h", authz: fakeAuthz{"c", "h"}, fail: true},
		{name: "dst and cur root across trees", from: "c", to: "h", authz: fakeAuthz{"a", "h"}},
		{name: "mrca in tree", from: "e", to: "g", authz: fakeAuthz{"d"}},
		{name: "parents in tree", from: "c", to: "g", authz: fakeAuthz{"c", "g"}, fail: true},
		{name: "parents in tree, missing intermediate", from: "c", to: "g", authz: fakeAuthz{"c", "g"}, unexist: []string{"b", "d"}, fail: true},
		{name: "parents in tree, missing ancestors", from: "c", to: "g", authz: fakeAuthz{"c", "g"}, unexist: []string{"a", "b", "d"}},
		{name: "parents in tree, missing root", from: "c", to: "g", authz: fakeAuthz{"c", "g"}, unexist: []string{"a"}, fail: true},
		{name: "grandparents in tree, missing root", from: "c", to: "g", authz: fakeAuthz{"b", "g"}, unexist: []string{"a"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			f := createTestForest(tc.unexist)
			foo := createNS(f, "foo", nil)
			foo.SetParent(f.Get(tc.from))
			h := &Hierarchy{Forest: f, authz: tc.authz}
			l := zap.Logger(false)

			// Create request
			hc := &api.HierarchyConfiguration{Spec: api.HierarchyConfigurationSpec{Parent: tc.to}}
			hc.ObjectMeta.Name = api.Singleton
			hc.ObjectMeta.Namespace = "foo"
			req := &request{hc: hc, ui: &authn.UserInfo{Username: "jen"}}

			// Test
			got := h.handle(context.Background(), l, req)

			// Report
			logResult(t, got.AdmissionResponse.Result)
			//reason := got.AdmissionResponse.Result.Reason
			g.Expect(got.AdmissionResponse.Allowed).ShouldNot(Equal(tc.fail))
		})
	}
}

// createTestForest creates the following forest:
// a -> b -> c -> foo
//   |
//   -> d -> e
//        |
//        -> g
// h
//
// The ue (UnExists) parameter prevents the specified namespaces from having SetExists called on
// them.
func createTestForest(ue []string) *forest.Forest {
	f := forest.NewForest()
	a := createNS(f, "a", ue)
	b := createNS(f, "b", ue)
	c := createNS(f, "c", ue)
	d := createNS(f, "d", ue)
	e := createNS(f, "e", ue)
	g := createNS(f, "g", ue)
	createNS(f, "h", ue)
	b.SetParent(a)
	c.SetParent(b)
	d.SetParent(a)
	e.SetParent(d)
	g.SetParent(d)
	return f
}

// createNS creates nnm and sets it to existing, unless it's listed in the ue (UnExists) list. Note
// that we can't call UnsetExists() since that also destroys the hierarchy and cleans it up.
func createNS(f *forest.Forest, nnm string, ue []string) *forest.Namespace {
	ns := f.Get(nnm)
	for _, u := range ue { // hey, it's fast _enough_ :)
		if u == nnm {
			return ns
		}
	}
	ns.SetExists()
	return ns
}

func logResult(t *testing.T, result *metav1.Status) {
	t.Logf("Got reason %q, code %d, msg %q", result.Reason, result.Code, result.Message)
}

// fakeAuthz implements authzClient. Any namespaces that are in the slice are allowed; anything else
// is denied.
type fakeAuthz []string

func (f fakeAuthz) IsAdmin(_ context.Context, _ *authn.UserInfo, nnm string) (bool, error) {
	for _, n := range f {
		if n == nnm {
			return true, nil
		}
	}
	return false, nil
}
