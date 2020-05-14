package forest

import (
	"sort"
	"testing"

	. "github.com/onsi/gomega"
)

func TestSetParentAndAncestorNames(t *testing.T) {
	tests := []struct {
		name  string
		chain []string
	}{
		{name: "no parent", chain: []string{"foo"}},
		{name: "two-level", chain: []string{"foo", "bar"}},
		{name: "cycle", chain: []string{"foo", "bar", "foo"}},
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
			g.Expect(last.AncestryNames()).Should(Equal(tc.chain))
		})
	}
}

func TestCycleNames(t *testing.T) {
	tests := []struct {
		name  string
		chain []string
		want  []string
	}{
		{name: "no parent", chain: []string{"foo"}},
		{name: "two-level", chain: []string{"foo", "bar"}},
		{name: "in cycle", chain: []string{"foo", "bar", "foo"}, want: []string{"bar", "foo", "bar"}}, // rotated so smallest name is first/last
		{name: "below cycle", chain: []string{"foo", "bar", "foo", "baz"}},                            // baz isn't in a cycle itself
		{name: "longer cycle ordered", chain: []string{"a", "c", "z", "a"}, want: []string{"a", "c", "z", "a"}},
		{name: "longer cycle unordered", chain: []string{"c", "z", "a", "c"}, want: []string{"a", "c", "z", "a"}},
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
			g.Expect(last.CycleNames()).Should(Equal(tc.want))
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
		sort.Strings(tc.want)
		t.Run(tc.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			root := f.Get(tc.root)
			g.Expect(root.DescendantNames()).Should(Equal(tc.want))
		})
	}
}
