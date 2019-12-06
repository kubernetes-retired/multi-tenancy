// Package forest defines the Forest type.
package forest

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
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
		forest:          f,
		name:            nm,
		children:        namedNamespaces{},
		conditions:      conditions{},
		originalObjects: objects{},
	}
	f.namespaces[nm] = ns
	return ns
}

type namedNamespaces map[string]*Namespace

// TODO Store source objects by GK in the forest - https://github.com/kubernetes-sigs/multi-tenancy/issues/281
type objects map[schema.GroupVersionKind]map[string]*unstructured.Unstructured
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

	// originalObjects store the objects created by users, identified by GVK and name.
	// It serves as the source of truth for object controllers to propagate objects.
	originalObjects objects

	// conditions store conditions so that object propagation can be disabled if there's a problem
	// on this namespace.
	conditions conditions

	// RequiredChildOf indicates that this namespace is being or was created solely to live as a
	// subnamespace of the specified parent.
	RequiredChildOf string
}

type condition struct {
	code api.Code
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
		if chain := p.AncestryNames(ns); chain != nil {
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
	for k := range ns.children {
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
	for k := range ns.children {
		a = append(a, k)
	}

	return a
}

// AncestryNames returns a slice of strings like ["grandparent", "parent", "child"] if there is
// a path from `other` to the current namespace (if `other` is nil, the first element of the slice
// will be the root of the tree, *not* the empty string).
func (ns *Namespace) AncestryNames(other *Namespace) []string {
	if ns == nil {
		// Nil forest has nil ancestry
		return nil
	}
	if ns == other || (ns.parent == nil && other == nil) {
		// Either we found `other` or the root
		return []string{ns.name}
	}
	if ns.parent == nil {
		// Ancestry to `other` doesn't exist
		return nil
	}
	ancestry := ns.parent.AncestryNames(other)
	if ancestry == nil {
		// Ancestry to `other` wasn't found
		return nil
	}

	// Add ourselves to the ancestry
	return append(ancestry, ns.name)
}

// SetOriginalObject updates or creates the original object in the namespace in the forest.
func (ns *Namespace) SetOriginalObject(obj *unstructured.Unstructured) {
	gvk := obj.GroupVersionKind()
	name := obj.GetName()
	_, ok := ns.originalObjects[gvk]
	if !ok {
		ns.originalObjects[gvk] = map[string]*unstructured.Unstructured{}
	}
	ns.originalObjects[gvk][name] = obj
}

// GetOriginalObject gets an original object from a key string. It returns nil, if the key doesn't exist.
func (ns *Namespace) GetOriginalObject(gvk schema.GroupVersionKind, key string) *unstructured.Unstructured {
	return ns.originalObjects[gvk][key]
}

// HasOriginalObject returns if the namespace has an original object.
func (ns *Namespace) HasOriginalObject(gvk schema.GroupVersionKind, oo string) bool {
	return ns.GetOriginalObject(gvk, oo) != nil
}

// DeleteOriginalObject deletes an original object from a key string.
func (ns *Namespace) DeleteOriginalObject(gvk schema.GroupVersionKind, key string) {
	delete(ns.originalObjects[gvk], key)
	// Garbage collection
	if len(ns.originalObjects[gvk]) == 0 {
		delete(ns.originalObjects, gvk)
	}
}

// GetOriginalObjects returns all original objects in the namespace.
func (ns *Namespace) GetOriginalObjects(gvk schema.GroupVersionKind) []*unstructured.Unstructured {
	o := []*unstructured.Unstructured{}
	for _, obj := range ns.originalObjects[gvk] {
		o = append(o, obj)
	}
	return o
}

// GetPropagatedObjects returns all original copies in the ancestors.
func (ns *Namespace) GetPropagatedObjects(gvk schema.GroupVersionKind) []*unstructured.Unstructured {
	o := []*unstructured.Unstructured{}
	ans := ns.AncestryNames(nil)
	for _, n := range ans {
		// Exclude the original objects in this namespace
		if n == ns.name {
			continue
		}
		o = append(o, ns.forest.Get(n).GetOriginalObjects(gvk)...)
	}
	return o
}

// GetSource returns the original copy in the ancestors if it exists.
// Otherwise, return nil.
func (ns *Namespace) GetSource(gvk schema.GroupVersionKind, name string) *unstructured.Unstructured {
	pos := ns.GetPropagatedObjects(gvk)
	for _, po := range pos {
		if po.GetName() == name {
			return po
		}
	}
	return nil
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

// HasLocalCritCondition returns if the namespace has any local critical condition.
func (ns *Namespace) HasLocalCritCondition() bool {
	return ns.GetConditions(Local) != nil
}

// ClearCritConditions clears local Crit conditions and CritAncestor conditions.
func (ns *Namespace) ClearCritConditions() {
	ns.clearConditions(Local)

	for key, conds := range ns.conditions {
		if len(conds) == 1 && conds[0].code == api.CritAncestor {
			ns.clearConditions(key)
		}
	}
}

// clearConditions clears conditions by key in a namespace.
func (ns *Namespace) clearConditions(key string) {
	delete(ns.conditions, key)
}

// GetConditions gets a condition list from a key string. It returns nil, if the key doesn't exist.
func (ns *Namespace) GetConditions(key string) []condition {
	c, _ := ns.conditions[key]
	return c
}

// SetCondition adds a condition into the list of conditions for key string, returning
// true if it does not exist previously.
func (ns *Namespace) SetCondition(key string, code api.Code, msg string) {
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
func (ns *Namespace) Conditions(log logr.Logger) []api.Condition {
	return flatten(ns.convertConditions(log))
}

// convertConditions converts string -> condition{code, msg} map into condition{code, msg} -> affected map.
func (ns *Namespace) convertConditions(log logr.Logger) map[api.Code]map[string][]api.AffectedObject {
	converted := map[api.Code]map[string][]api.AffectedObject{}
	for key, conditions := range ns.conditions {
		for _, condition := range conditions {
			affectedObject := getAffectedObject(key, log)
			if affected, ok := converted[condition.code][condition.msg]; !ok {
				converted[condition.code] = map[string][]api.AffectedObject{condition.msg: {affectedObject}}
			} else {
				converted[condition.code][condition.msg] = append(affected, affectedObject)
			}
		}
	}
	return converted
}

// flatten flattens condition{code, msg} -> affected map into a list of condition{code, msg, []affected}.
func flatten(m map[api.Code]map[string][]api.AffectedObject) []api.Condition {
	flattened := []api.Condition{}
	for code, msgAffected := range m {
		for msg, affected := range msgAffected {
			// Local conditions do not need Affects.
			if len(affected) == 1 && (api.AffectedObject{}) == affected[0] {
				flattened = append(flattened, api.Condition{Code: code, Msg: msg})
			} else {
				flattened = append(flattened, api.Condition{Code: code, Msg: msg, Affects: affected})
			}
		}
	}
	if len(flattened) > 0 {
		return flattened
	}
	return nil
}

// getAffectedObject gets AffectedObject from a namespace or a string of form group/version/kind/namespace/name.
func getAffectedObject(gvknn string, log logr.Logger) api.AffectedObject {
	if gvknn == Local {
		return api.AffectedObject{}
	}

	a := strings.Split(gvknn, "/")
	// The affected is a namespace.
	if len(a) == 1 {
		return api.AffectedObject{Namespace: a[0]}
	}
	// The affected is an object.
	if len(a) == 5 {
		return api.AffectedObject{
			Group:     a[0],
			Version:   a[1],
			Kind:      a[2],
			Namespace: a[3],
			Name:      a[4],
		}
	}

	// Return an invalid object with key as name if the key is invalid.
	log.Info("Invalid key for the affected object", "key", gvknn)
	return api.AffectedObject{
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
		childNs := ns.forest.Get(child)
		descendantsOfChild := childNs.DescendantNames()
		descendants = append(descendants, descendantsOfChild...)
	}
	return descendants
}
