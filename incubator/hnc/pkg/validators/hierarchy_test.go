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
		server fakeServer
		forest string
		from   string
		to     string
		code   int32
	}{
		{name: "nothing in tree", forest: "-aa", from: "b", to: "c", code: 401},                                              // a <- (b, c)
		{name: "root in tree", forest: "-aa", from: "b", to: "c", server: "a"},                                               // a <- (b, c)
		{name: "parents but not root", forest: "-aab", from: "d", to: "c", server: "bc", code: 401},                          // a <- (b <- d, c)
		{name: "dst only across trees", forest: "--", from: "a", to: "b", server: "b", code: 401},                            // a; b
		{name: "cur root only across trees", forest: "--", from: "a", to: "b", server: "a", code: 401},                       // a; b
		{name: "dst and cur parent (but not root) across trees", forest: "-a-", from: "b", to: "c", server: "bc", code: 401}, // a <- b; c
		{name: "dst and cur root across trees", forest: "-a-", from: "b", to: "c", server: "ac"},                             // a <- b; c
		{name: "mrca in tree", forest: "-abb", from: "c", to: "d", server: "b"},                                              // a <- b <- (c, d)
		{name: "dest but unsynced parent", forest: "-", from: "z", to: "a", server: "a", code: 503},                          // a (z exists on the server)
		{name: "dest but missing parent", forest: "-", from: "z", to: "a", server: "a:z"},                                    // a (z is missing)
		{name: "dest but missing ancestor", forest: "z-", from: "a", to: "b", server: "ab", code: 403},                       // z <- a; b (z is missing)
	}
	for _, tc := range tests {
		t.Run("permission on "+tc.name, func(t *testing.T) {
			// Setup
			g := NewGomegaWithT(t)
			f := foresttest.Create(tc.forest)
			foo := f.Get("foo")
			foo.SetExists()
			p := f.Get(tc.from)
			foo.SetParent(p)
			if !p.Exists() {
				foo.SetLocalCondition(api.CritParentMissing, "missing")
			}
			h := &Hierarchy{Forest: f, server: tc.server}
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
			g.Expect(got.AdmissionResponse.Result.Code).Should(Equal(tc.code))
		})
	}
}

func logResult(t *testing.T, result *metav1.Status) {
	t.Logf("Got reason %q, code %d, msg %q", result.Reason, result.Code, result.Message)
}

// fakeServer implements serverClient. It's implemented as a string separated by a colon (":") with
// the following meanings:
// * Anything *before* the colon passes the IsAdmin check
// * Anything *after* the colon *fails* the Exists check
// If the colon is missing, it's assumed to come at the end of the string
type fakeServer string

func (f fakeServer) IsAdmin(_ context.Context, _ *authn.UserInfo, nnm string) (bool, error) {
	for _, n := range f {
		if nnm == string(n) {
			return true, nil
		}
		if n == ':' {
			return false, nil
		}
	}
	return false, nil
}

func (f fakeServer) Exists(_ context.Context, nnm string) (bool, error) {
	foundColon := false
	for _, n := range f {
		if n == ':' {
			foundColon = true
			continue
		}
		if !foundColon {
			continue
		}
		if nnm == string(n) {
			return false, nil
		}
	}
	return true, nil
}
