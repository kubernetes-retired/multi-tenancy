package forest

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestGetAncestoryNames(t *testing.T) {
	tests := []struct {
		name  string
		chain []string
		to    string
		want  []string
	}{
		{name: "no parent to root", chain: []string{"foo"}, want: []string{"foo"}},
		{name: "no parent to self", chain: []string{"foo"}, to: "foo", want: []string{"foo"}},
		{name: "two-level to root", chain: []string{"foo", "bar"}, want: []string{"foo", "bar"}},
		{name: "two-level to top", chain: []string{"foo", "bar"}, to: "foo", want: []string{"foo", "bar"}},
		{name: "two-level to bottom", chain: []string{"foo", "bar"}, to: "bar", want: []string{"bar"}},
		{name: "three-level to mid", chain: []string{"foo", "bar", "baz"}, to: "bar", want: []string{"bar", "baz"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			// Create the chain
			f := NewForest()
			var last *Namespace
			for _, nm := range tc.chain {
				parent := last
				last = f.AddOrGet(nm)
				last.SetParent(parent)
			}
			to := f.Get(tc.to)
			g.Expect(last.GetAncestoryNames(to)).Should(Equal(tc.want))
		})
	}
}

func TestSetParent(t *testing.T) {
	tests := []struct {
		name   string
		chain  []string
		child  string
		parent string
		fail   bool
	}{
		{name: "set nil parent", chain: []string{"foo"}, child: "foo"},
		{name: "move to grandparent", chain: []string{"foo", "bar", "baz"}, child: "baz", parent: "foo"},
		{name: "set self parent", chain: []string{"foo"}, child: "foo", parent: "foo", fail: true},
		{name: "create cycle", chain: []string{"foo", "bar", "baz"}, child: "foo", parent: "baz", fail: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create the chain
			f := NewForest()
			var last *Namespace
			for _, nm := range tc.chain {
				parent := last
				last = f.AddOrGet(nm)
				last.SetParent(parent)
			}
			child := f.Get(tc.child)
			t.Logf("Old ancestry is: %v", child.GetAncestoryNames(nil))
			parent := f.Get(tc.parent)
			err := child.SetParent(parent)
			if err == nil {
				t.Logf("New ancestry is: %v", child.GetAncestoryNames(nil))
				if tc.fail {
					t.Error("got success, want failure")
				}
			} else {
				if tc.fail {
					t.Logf("Got error as expected: %v", err)
				} else {
					t.Errorf("Got error %q, want success", err)
				}
			}
		})
	}
}
