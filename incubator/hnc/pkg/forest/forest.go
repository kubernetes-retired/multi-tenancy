// Package forest defines the Forest type.
package forest

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/go-logr/logr"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
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
	ns = &Namespace{
		forest:     f,
		name:       nm,
		children:   namedNamespaces{},
		conditions: conditions{},
	}
	f.namespaces[nm] = ns
	return ns
}

type namedNamespaces map[string]*Namespace
type conditions map[string][]condition

// Local represents conditions that originated from this namespace
const Local = ""

// Namespace represents a namespace in a forest. Other than its structure, it contains some
// properties useful to the reconcilers.
type Namespace struct {
	forest   *Forest
	name     string
	parent   *Namespace
	children namedNamespaces
	exists   bool

	// conditions store conditions so that object propagation can be disabled if there's a problem
	// on this namespace.
	conditions conditions

	// RequiredChildOf indicates that this namespace is being or was created solely to live as a
	// subnamespace of the specified parent.
	RequiredChildOf string
}

type condition struct {
	code tenancy.Code
	msg  string
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
	if reason := ns.CanSetParent(p); reason != "" {
		return errors.New(reason)
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

// CanSetParent returns the empty string if the assignment is currently legal, or a non-empty string
// indicating the reason if it cannot be done.
func (ns *Namespace) CanSetParent(p *Namespace) string {
	// Check for cycles
	if p != nil {
		// Simple case
		if p == ns {
			return fmt.Sprintf("%q cannot be set as its own parent", p.name)
		}
		if chain := p.AncestoryNames(ns); chain != nil {
			return fmt.Sprintf("cycle when making %q the parent of %q: current ancestry is %q",
				p.name, ns.name, strings.Join(chain, " <- "))
		}
	}

	return ""
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
	if ns == nil {
		// Nil forest has nil ancestory
		return nil
	}
	if ns == other || (ns.parent == nil && other == nil) {
		// Either we found `other` or the root
		return []string{ns.name}
	}
	if ns.parent == nil {
		// Ancestory to `other` doesn't exist
		return nil
	}
	ancestory := ns.parent.AncestoryNames(other)
	if ancestory == nil {
		// Ancestory to `other` wasn't found
		return nil
	}

	// Add ourselves to the ancestory
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

// HasCondition returns if the namespace has any local condition.
func (ns *Namespace) HasCondition() bool {
	return len(ns.conditions) > 0
}

// HasCritCondition returns if the namespace has any critical condition.
func (ns *Namespace) HasCritCondition() bool {
	// For now, all the critical conditions are set locally. It may not be true for
	// future critical conditions. We may not want to just use local conditions.
	return ns.GetCondition(Local) != nil
}

// ClearConditions clears local conditions in the namespace.
func (ns *Namespace) ClearConditions(key string) {
	delete(ns.conditions, key)
}

// GetCondition gets a condition list from a key string. It returns nil, if the key doesn't exist.
func (ns *Namespace) GetCondition(key string) []condition {
	c, _ := ns.conditions[key]
	return c
}

// SetCondition adds a condition into the list of conditions for key string, returning
// true if it does not exist previously.
func (ns *Namespace) SetCondition(key string, code tenancy.Code, msg string) {
	oldConditions := ns.conditions[key]
	for _, condition := range oldConditions {
		if condition.code == code && condition.msg == msg {
			return
		}
	}

	ns.conditions[key] = append(oldConditions, condition{code: code, msg: msg})
}

// Conditions returns a list of conditions in the namespace.
// It converts map[string][]condition into []Condition.
func (ns *Namespace) Conditions(log logr.Logger) []tenancy.Condition {
	return flatten(ns.convertConditions(log))
}

// convertConditions converts string -> condition{code, msg} map into condition{code, msg} -> affected map.
func (ns *Namespace) convertConditions(log logr.Logger) map[tenancy.Code]map[string][]tenancy.AffectedObject {
	converted := map[tenancy.Code]map[string][]tenancy.AffectedObject{}
	for key, conditions := range ns.conditions {
		for _, condition := range conditions {
			affectedObject := getAffectedObject(key, log)
			if affected, ok := converted[condition.code][condition.msg]; !ok {
				converted[condition.code] = map[string][]tenancy.AffectedObject{condition.msg: {affectedObject}}
			} else {
				converted[condition.code][condition.msg] = append(affected, affectedObject)
			}
		}
	}
	return converted
}

// flatten flattens condition{code, msg} -> affected map into a list of condition{code, msg, []affected}.
func flatten(m map[tenancy.Code]map[string][]tenancy.AffectedObject) []tenancy.Condition {
	flattened := []tenancy.Condition{}
	for code, msgAffected := range m {
		for msg, affected := range msgAffected {
			// Local conditions do not need Affects.
			if len(affected) == 1 && (tenancy.AffectedObject{}) == affected[0] {
				flattened = append(flattened, tenancy.Condition{Code: code, Msg: msg})
			} else {
				flattened = append(flattened, tenancy.Condition{Code: code, Msg: msg, Affects: affected})
			}
		}
	}
	if len(flattened) > 0 {
		return flattened
	}
	return nil
}

// getAffectedObject gets AffectedObject from a namespace or a string of form group/version/kind/namespace/name.
func getAffectedObject(gvknn string, log logr.Logger) tenancy.AffectedObject {
	if gvknn == Local {
		return tenancy.AffectedObject{}
	}

	a := strings.Split(gvknn, "/")
	// The affected is a namespace.
	if len(a) == 1 {
		return tenancy.AffectedObject{Namespace: a[0]}
	}
	// The affected is an object.
	if len(a) == 5 {
		return tenancy.AffectedObject{
			Group:     a[0],
			Version:   a[1],
			Kind:      a[2],
			Namespace: a[3],
			Name:      a[4],
		}
	}

	// Return an invalid object with key as name if the key is invalid.
	log.Info("Invalid key for the affected object", "key", gvknn)
	return tenancy.AffectedObject{
		Group:     "",
		Version:   "",
		Kind:      "",
		Namespace: "INVALID OBJECT",
		Name:      gvknn,
	}
}

// DescendantNames returns a slice of strings like ["child" ... "grandchildren" ...] of
// names of all namespaces in its subtree. Nil is returned if the namespace has no descendant.
func (ns *Namespace) DescendantNames() []string {
	children := ns.ChildNames()
	descendants := children
	for _, child := range children {
		child_ns := ns.forest.Get(child)
		descendantsOfChild := child_ns.DescendantNames()
		descendants = append(descendants, descendantsOfChild...)
	}
	return descendants
}
