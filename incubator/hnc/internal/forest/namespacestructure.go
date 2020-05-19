package forest

import (
	"fmt"
	"sort"
	"strings"
)

// ChildNames returns a sorted list of names or nil if there are no children.
func (ns *Namespace) ChildNames() []string {
	if len(ns.children) == 0 {
		return nil
	}
	nms := []string{}
	for k := range ns.children {
		nms = append(nms, k)
	}
	sort.Strings(nms)
	return nms
}

// IsAncestor is *not* cycle-safe, so should only be called from namespace trees that are known not
// to have cycles.
func (ns *Namespace) IsAncestor(other *Namespace) bool {
	if ns.parent == other {
		return true
	}
	if ns.parent == nil {
		return false
	}
	return ns.parent.IsAncestor(other)
}

// SetParent modifies the namespace's parent, including updating the list of children. It may result
// in a cycle being created; this can be prevented by calling CanSetParent before, or seeing if it
// happened by calling CycleNames afterwards.
func (ns *Namespace) SetParent(p *Namespace) {
	// Remove old parent and cleans it up.
	if ns.parent != nil {
		delete(ns.parent.children, ns.name)
		if len(ns.parent.children) == 0 {
			ns.parent.clean()
		}
	}

	// Update new parent.
	ns.parent = p
	if p != nil {
		p.children[ns.name] = ns
	}
}

// CanSetParent returns the empty string if the assignment is currently legal, or a non-empty string
// indicating the reason if it cannot be done.
func (ns *Namespace) CanSetParent(p *Namespace) string {
	if p == nil {
		return ""
	}

	// Simple case
	if p == ns {
		return fmt.Sprintf("%q cannot be set as its own parent", p.name)
	}

	// Check for cycles; see if the current namespace (the proposed child) is already an ancestor of
	// the proposed parent. Start at the end of the ancestry (e.g. at the proposed parent) and work
	// our way up to the root.
	ancestors := p.AncestryNames()
	cycle := []string{}
	found := false
	for i := len(ancestors) - 1; !found && i >= 0; i-- {
		cycle = append(cycle, ancestors[i])
		found = (ancestors[i] == ns.name)
	}
	if found {
		return fmt.Sprintf("cycle when making %q the parent of %q: current ancestry is %s",
			p.name, ns.name, strings.Join(cycle, " -> "))
	}

	return ""
}

// RelativesNames returns the children and parent.
func (ns *Namespace) RelativesNames() []string {
	a := []string{}
	if ns.parent != nil {
		a = append(a, ns.parent.name)
	}
	for k := range ns.children {
		a = append(a, k)
	}

	return a
}

// AncestryNames returns all ancestors of this namespace. The namespace itself is the last element
// of the returned slice, with the root at the beginning of the list.
//
// This method is cycle-safe, and can be used to detect and recover from cycles. If there's a cycle,
// the first ancestor that's a member of the cycle we encounter is repeated at the beginning of the
// list.
func (ns *Namespace) AncestryNames() []string {
	if ns == nil {
		return nil
	}

	cycleCheck := map[string]bool{ns.name: true}
	ancestors := []string{ns.name}
	anc := ns.parent
	for anc != nil {
		ancestors = append([]string{anc.name}, ancestors...)
		if cycleCheck[anc.name] {
			return ancestors
		}
		cycleCheck[anc.name] = true
		anc = anc.parent
	}

	return ancestors
}

// CycleNames returns nil if the namespace is not in a cycle, or a list of names in the cycle if
// it is. All namespaces in the cycle return the same list, which is the same as calling
// ns.AncestryNames() on the namespaces with the lexicographically smallest name.
func (ns *Namespace) CycleNames() []string {
	// If this namespaces is *in* a cycle, it will be the first repeated element encountered by
	// AncestryNames(), and therefore will be both the first and the last element.
	ancestors := ns.AncestryNames()
	if len(ancestors) == 1 || ancestors[0] != ns.name {
		return nil
	}
	ancestors = ancestors[1:] // don't need the repeated element

	// Find the smallest name and where it is
	sidx := 0
	snm := ancestors[0]
	for idx, nm := range ancestors {
		if nm < snm {
			sidx = idx
			snm = nm
		}
	}

	// Rotate the slice, and then duplicate the smallest element
	ancestors = append(ancestors[sidx:], ancestors[:sidx]...)
	return append(ancestors, snm)
}

// DescendantNames returns a slice of strings like ["achild", "agrandchild", "bchild", ...] of names
// of all namespaces in its subtree, or nil if the namespace has no descendents. The names are
// returned in alphabetical order (as defined by `sort.Strings()`), *not* depth-first,
// breadth-first, etc.
//
// This method is cycle-safe. If there are cycles, each namespace is only listed once.
func (ns *Namespace) DescendantNames() []string {
	ds := map[string]bool{}
	ns.populateDescendants(ds)
	if len(ds) == 0 {
		return nil
	}
	d := []string{}
	for k, _ := range ds {
		d = append(d, k)
	}
	sort.Strings(d)
	return d
}

// populateDescendants is a cycle-safe way of finding all descendants of a namespace. If any
// namespace turns out to be its own descendant, it's skipped on subsequent encounters.
func (ns *Namespace) populateDescendants(d map[string]bool) {
	for _, c := range ns.ChildNames() {
		if d[c] {
			continue
		}
		d[c] = true
		cns := ns.forest.Get(c)
		cns.populateDescendants(d)
	}
}
