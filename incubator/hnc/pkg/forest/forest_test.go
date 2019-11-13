package forest

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestAncestryNames(t *testing.T) {
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
				last = f.Get(nm)
				last.SetParent(parent)
			}
			to := f.Get(tc.to)
			g.Expect(last.AncestryNames(to)).Should(Equal(tc.want))
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
				last = f.Get(nm)
				last.SetParent(parent)
			}
			child := f.Get(tc.child)
			t.Logf("Old ancestry is: %v", child.AncestryNames(nil))
			parent := f.Get(tc.parent)
			err := child.SetParent(parent)
			if err == nil {
				t.Logf("New ancestry is: %v", child.AncestryNames(nil))
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

func TestDescendantNames(t *testing.T) {
	// Test a tree has the following structure:
	//         a
	//       /  \
	//     /     \
	//    b       c
	//  / | \    /  \
	// x  y z  123 456
	//          |
	//         789
	kinship := map[string][]string{
		"a":   []string{"b", "c"},
		"b":   []string{"x", "y", "z"},
		"c":   []string{"123", "456"},
		"123": []string{"789"},
	}

	// Create the forest
	f := NewForest()
	for parentName, childrenNames := range kinship {
		parent := f.Get(parentName)
		for _, childName := range childrenNames {
			child := f.Get(childName)
			child.SetParent(parent)
		}
	}

	tests := []struct {
		name string
		root string
		want []string
	}{
		{name: "no descendant", root: "456", want: nil},
		{name: "one descendant", root: "123", want: []string{"789"}},
		{name: "one-level descendants", root: "b", want: []string{"x", "y", "z"}},
		{name: "two-level descendants", root: "c", want: []string{"123", "456", "789"}},
		{name: "three-level descendants", root: "a", want: []string{"b", "c", "x", "y", "z", "123", "456", "789"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			root := f.Get(tc.root)
			g.Expect(root.DescendantNames()).Should(Equal(tc.want))
		})
	}
}
