package validators

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	authn "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/foresttest"
)

func TestStructure(t *testing.T) {
	f := foresttest.Create("-a-") // a <- b; c
	h := &Hierarchy{Forest: f}
	l := zap.Logger(false)

	tests := []struct {
		name string
		nnm  string
		pnm  string
		fail bool
	}{
		{name: "ok", nnm: "a", pnm: "c"},
		{name: "missing parent", nnm: "a", pnm: "brumpf", fail: true},
		{name: "self-cycle", nnm: "a", pnm: "a", fail: true},
		{name: "other cycle", nnm: "a", pnm: "b", fail: true},
		{name: "exclude kube-system", nnm: "a", pnm: "kube-system", fail: true},
		{name: "exclude kube-public", nnm: "a", pnm: "kube-public", fail: true},
		{name: "exclude hnc-system", nnm: "a", pnm: "hnc-system", fail: true},
		{name: "exclude cert-manager", nnm: "a", pnm: "cert-manager", fail: true},
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
		name   string
		authz  fakeAuthz
		forest string
		from   string
		to     string
		fail   bool
	}{
		{name: "nothing in tree", forest: "-aa", from: "b", to: "c", fail: true},                                             // a <- (b, c)
		{name: "root in tree", forest: "-aa", from: "b", to: "c", authz: "a"},                                                // a <- (b, c)
		{name: "parents but not root", forest: "-aab", from: "d", to: "c", authz: "bc", fail: true},                          // a <- (b <- d, c)
		{name: "dst only across trees", forest: "--", from: "a", to: "b", authz: "b", fail: true},                            // a; b
		{name: "cur root only across trees", forest: "--", from: "a", to: "b", authz: "a", fail: true},                       // a; b
		{name: "dst and cur parent (but not root) across trees", forest: "-a-", from: "b", to: "c", authz: "bc", fail: true}, // a <- b; c
		{name: "dst and cur root across trees", forest: "-a-", from: "b", to: "c", authz: "ac"},                              // a <- b; c
		{name: "mrca in tree", forest: "-abb", from: "c", to: "d", authz: "b"},                                               // a <- b <- (c, d)
	}
	for _, tc := range tests {
		t.Run("permission on "+tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			f := foresttest.Create(tc.forest)
			foo := f.Get("foo")
			foo.SetExists()
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

func logResult(t *testing.T, result *metav1.Status) {
	t.Logf("Got reason %q, code %d, msg %q", result.Reason, result.Code, result.Message)
}

// fakeAuthz implements authzClient. Any namespaces that are in the slice are allowed; anything else
// is denied.
type fakeAuthz string

func (f fakeAuthz) IsAdmin(_ context.Context, _ *authn.UserInfo, nnm string) (bool, error) {
	for _, n := range f {
		if nnm == string(n) {
			return true, nil
		}
	}
	return false, nil
}
