// Package forest defines the Forest type.
package forest

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Forest defines a forest of namespaces - that is, a set of trees. It includes methods to mutate
// the forest legally (ie, prevent cycles).
//
// The forest should always be locked/unlocked (via the `Lock` and `Unlock` methods) while it's
// being mutated to avoid different controllers from making inconsistent changes.
type Forest struct {
	lock       sync.Mutex
	namespaces namedNamespaces
}

func NewForest() *Forest {
	return &Forest{
		lock:       sync.Mutex{},
		namespaces: namedNamespaces{},
	}
}

func (f *Forest) Lock() {
	f.lock.Lock()
}

func (f *Forest) Unlock() {
	f.lock.Unlock()
}

// Get returns a `Namespace` object representing a namespace in K8s.
func (f *Forest) Get(nm string) *Namespace {
	if nm == "" {
		// Impossible in normal circumstances, K8s doesn't allow unnamed objects. This should probably
		// be a panic since most clients won't be checking for nil, but it makes some scenarios easier
		// (ie "no parent" is returned as an empty string, which really should be represented as a nil
		// pointer) so let's leave this as-is for now.
		return nil
	}
	ns, ok := f.namespaces[nm]
	if ok {
		return ns
	}
	ns = &Namespace{forest: f, name: nm, children: namedNamespaces{}}
	f.namespaces[nm] = ns
	return ns
}

type namedNamespaces map[string]*Namespace

// Namespace represents a namespace in a forest. Other than its structure, it contains some
// properties useful to the reconcilers.
type Namespace struct {
	forest   *Forest
	name     string
	parent   *Namespace
	children namedNamespaces
	exists   bool
}

// Exists returns true if the namespace exists.
func (ns *Namespace) Exists() bool {
	return ns.exists
}

// SetExists marks this namespace as existing, returning true if didn't previously exist.
func (ns *Namespace) SetExists() bool {
	changed := !ns.exists
	ns.exists = true
	return changed
}

// UnsetExists marks this namespace as missing, returning true if it previously existed. It also
// removes it from its parent, if any, since a nonexistent namespace can't have a parent.
func (ns *Namespace) UnsetExists() bool {
	changed := ns.exists
	ns.SetParent(nil) // Unreconciled namespaces can't specify parents
	ns.exists = false
	ns.clean() // clean up if this is a useless data structure
	return changed
}

// clean garbage collects this namespace if it has a zero value.
func (ns *Namespace) clean() {
	// Don't clean up something that either exists or is otherwise referenced.
	if ns.exists || len(ns.children) > 0 {
		return
	}

	// Remove from the forest.
	delete(ns.forest.namespaces, ns.name)
}

// SetParent attempts to set the namespace's parent. This includes removing it from the list of
// children of its own parent, if necessary. It may return an error if the parent is illegal, i.e.
// if it causes a cycle. It cannot cause an error if the parent is being set to nil.
func (ns *Namespace) SetParent(p *Namespace) error {
	// Check for cycles
	if p != nil {
		// Simple case
		if p == ns {
			return fmt.Errorf("%q cannot be set as its own parent", p.name)
		}
		if chain := p.AncestoryNames(ns); chain != nil {
			return fmt.Errorf("cycle when making %q the parent of %q: current ancestry is %q",
				p.name, ns.name, strings.Join(chain, " <- "))
		}
	}

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
	return nil
}

func (ns *Namespace) Name() string {
	if ns == nil {
		return "<none>"
	}
	return ns.name
}

func (ns *Namespace) Parent() *Namespace {
	return ns.parent
}

// ChildNames returns a sorted list of names or nil if there are no children.
func (ns *Namespace) ChildNames() []string {
	if len(ns.children) == 0 {
		return nil
	}
	nms := []string{}
	for k, _ := range ns.children {
		nms = append(nms, k)
	}
	sort.Strings(nms)
	return nms
}

// RelativesNames returns the children and parent.
func (ns *Namespace) RelativesNames() []string {
	a := []string{}
	if ns.parent != nil {
		a = append(a, ns.parent.name)
	}
	for k, _ := range ns.children {
		a = append(a, k)
	}

	return a
}

// AncestoryNames returns a slice of strings like ["grandparent", "parent", "child"] if there is
// a path from `other` to the current namespace (if `other` is nil, the first element of the slice
// will be the root of the tree, *not* the empty string).
func (ns *Namespace) AncestoryNames(other *Namespace) []string {
	if ns == other || (ns.parent == nil && other == nil) {
		return []string{ns.name}
	}
	if ns.parent == nil {
		return nil
	}
	ancestory := ns.parent.AncestoryNames(other)
	if ancestory == nil {
		return nil
	}
	return append(ancestory, ns.name)
}

func (ns *Namespace) IsAncestor(other *Namespace) bool {
	if ns.parent == other {
		return true
	}
	if ns.parent == nil {
		return false
	}
	return ns.parent.IsAncestor(other)
}
