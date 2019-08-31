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

// AddOrGet returns a `Namespace` object representing an _existing_ namespace in K8s. Since it
// already exists, that means that it must be legal according to K8s (ie be unique, have a
// reasonable name, etc), so this method does very little checking.
func (f *Forest) AddOrGet(nm string) *Namespace {
	if nm == "" {
		// Basically impossible, K8s doesn't allow unnamed objects
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

// Get returns a `Namespace` object if it exists, or nil if it's missing.
func (f *Forest) Get(nm string) *Namespace {
	ns, _ := f.namespaces[nm]
	return ns
}

// Remove deletes the given namespace and returns a list of all namespaces who
// have been affected (ie, the parent and the now-orphaned children).
func (f *Forest) Remove(nm string) []string {
	affected := []string{}
	ns, _ := f.namespaces[nm]
	if ns == nil {
		return nil
	}

	// Update the parent, if any
	p := ns.Parent()
	if p != nil {
		affected = append(affected, p.name)
		delete(p.children, nm)
	}

	// Unset the parent from all children, if any
	for _, c := range ns.children {
		affected = append(affected, c.name)
		c.parent = nil
	}

	// Delete and return affected
	delete(f.namespaces, nm)
	return affected
}

type namedNamespaces map[string]*Namespace

// Namespace represents a namespace in a forest. For now, its only properties are its own name, its
// parent, and its children.
type Namespace struct {
	forest   *Forest
	name     string
	parent   *Namespace
	children namedNamespaces
}

// SetParent attempts to set the namespace's parent. This includes removing it from the list of
// children of its own parent, if necessary. It may return an error if the parent is illegal, i.e.
// if it causes a cycle.
func (ns *Namespace) SetParent(p *Namespace) error {
	// Check for cycles
	if p != nil {
		// Simple case
		if p == ns {
			return fmt.Errorf("%q cannot be set as its own parent", p.name)
		}
		if chain := p.GetAncestoryNames(ns); chain != nil {
			return fmt.Errorf("cycle when making %q the parent of %q: current ancestry is %q",
				p.name, ns.name, strings.Join(chain, " <- "))
		}
	}

	// Update the data structures
	if ns.parent != nil {
		delete(ns.parent.children, ns.name)
	}
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

func (ns *Namespace) ChildNames() []string {
	nms := []string{}
	for k, _ := range ns.children {
		nms = append(nms, k)
	}
	sort.Strings(nms)
	return nms
}

// GetAncestoryNames returns a slice of strings like ["grandparent", "parent", "child"] if there is
// a path from `other` to the current namespace (if `other` is nil, the first element of the slice
// will be the root of the tree, *not* the empty string).
func (ns *Namespace) GetAncestoryNames(other *Namespace) []string {
	if ns == other || (ns.parent == nil && other == nil) {
		return []string{ns.name}
	}
	if ns.parent == nil {
		return nil
	}
	ancestory := ns.parent.GetAncestoryNames(other)
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
