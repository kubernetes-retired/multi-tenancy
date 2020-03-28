// Package forest defines the Forest type.
package forest

import (
	"context"
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

var (
	// OutOfSync is used to report a precondition failure. It's not (currently) returned from this
	// package but is used externally.
	OutOfSync = errors.New("The forest is out of sync with itself")
)

// TypeSyncer syncs objects of a specific type. Reconcilers implement the interface so that they can be
// called by the HierarchyReconciler if the hierarchy changes.
type TypeSyncer interface {
	// SyncNamespace syncs objects of a namespace for a specific type.
	SyncNamespace(context.Context, logr.Logger, string) error
	// Provides the GVK that is handled by the reconciler who implements the interface.
	GetGVK() schema.GroupVersionKind
	// SetMode sets the propagation mode of objects that are handled by the reconciler who implements the interface.
	// The method also syncs objects in the cluster for the type handled by the reconciler if necessary.
	SetMode(context.Context, api.SynchronizationMode, logr.Logger) error
	// GetMode gets the propagation mode of objects that are handled by the reconciler who implements the interface.
	GetMode() api.SynchronizationMode
	// GetNumPropagatedObjects returns the number of propagated objects on the apiserver.
	GetNumPropagatedObjects() int
}

// NumPropagatedObjectsSyncer syncs the number of propagated objects. ConfigReconciler implements the
// interface so that it can be called by an ObjectReconciler if the number of propagated objects is changed.
type NumPropagatedObjectsSyncer interface {
	SyncNumPropagatedObjects(logr.Logger)
}

// Forest defines a forest of namespaces - that is, a set of trees. It includes methods to mutate
// the forest legally (ie, prevent cycles).
//
// The forest should always be locked/unlocked (via the `Lock` and `Unlock` methods) while it's
// being mutated to avoid different controllers from making inconsistent changes.
type Forest struct {
	lock       sync.Mutex
	namespaces namedNamespaces

	// types is a list of other reconcilers that HierarchyReconciler can call if the hierarchy
	// changes. This will force all objects to be re-propagated.
	//
	// This is probably wildly inefficient, and we can probably make better use of things like
	// owner references to make this better. But for a PoC, it works just fine.
	//
	// We put the list in the forest because the access to the list is guarded by the forest lock.
	// We can also move the lock out of the forest and pass it to all reconcilers that need the lock.
	// In that way, we don't need to put the list in the forest.
	types []TypeSyncer

	// ObjectsStatusSyncer is the ConfigReconciler that an object reconciler can call if the status of the HNCConfiguration
	// object needs to be updated.
	ObjectsStatusSyncer NumPropagatedObjectsSyncer
}

func NewForest() *Forest {
	return &Forest{
		namespaces: namedNamespaces{},
		types:      []TypeSyncer{},
	}
}

func (f *Forest) Lock() {
	f.lock.Lock()
}

func (f *Forest) Unlock() {
	f.lock.Unlock()
}

// AddTypeSyncer adds a reconciler to the types list.
func (f *Forest) AddTypeSyncer(nss TypeSyncer) {
	f.types = append(f.types, nss)
}

// GetTypeSyncer returns the reconciler for the given GVK or nil if the reconciler
// does not exist.
func (f *Forest) GetTypeSyncer(gvk schema.GroupVersionKind) TypeSyncer {
	for _, t := range f.types {
		if t.GetGVK() == gvk {
			return t
		}
	}
	return nil
}

// GetTypeSyncers returns the types list.
// Retuns a copy here so that the caller does not need to hold the mutex while accessing the returned value and can modify the
// returned value without fear of corrupting the original types list.
func (f *Forest) GetTypeSyncers() []TypeSyncer {
	types := make([]TypeSyncer, len(f.types))
	copy(types, f.types)
	return types
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

// GetNamespaceNames returns names of all namespaces in the cluster.
func (f *Forest) GetNamespaceNames() []string {
	var keys []string
	for k := range f.namespaces {
		keys = append(keys, k)
	}
	return keys
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

	// Todo remove Owner field or replace it with isOwned bool field. See issue:
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/552
	// Owner indicates that this namespace is being or was created solely to live as a
	// subnamespace of the specified parent.
	Owner string

	// HNSes store a list of HNS instances in the namespace.
	HNSes []string
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

// SetHNSes updates the HNSes and returns a difference between the new/old list.
func (ns *Namespace) SetHNSes(hnsnms []string) (diff []string) {
	add := make(map[string]bool)
	for _, nm := range hnsnms {
		add[nm] = true
	}
	for _, nm := range ns.HNSes {
		if add[nm] {
			delete(add, nm)
		} else {
			// This old HNS is not in the new HNS list.
			diff = append(diff, nm)
		}
	}

	for nm, _ := range add {
		// This new HNS is not in the old HNS list.
		diff = append(diff, nm)
	}

	ns.HNSes = hnsnms
	return
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

// HasLocalCritCondition returns if the namespace itself has any local critical conditions, ignoring
// its ancestors.
func (ns *Namespace) HasLocalCritCondition() bool {
	return ns.GetCondition(Local) != nil
}

// HasCritCondition returns if the namespace or any of its ancestors has any critical condition.
func (ns *Namespace) HasCritCondition() bool {
	if ns.HasLocalCritCondition() {
		return true
	}
	if ns.Parent() == nil {
		return false
	}
	return ns.Parent().HasCritCondition()
}

// ClearConditions clears local conditions in the namespace for a single key. If `code` is
// non-empty, it only clears conditions with that code, otherwise it clears all conditions for that
// key. It should only be called by the code that also *sets* the conditions.
//
// It returns true if it made any changes, false otherwise.
func (ns *Namespace) ClearConditions(key string, code api.Code) bool {
	// We don't *need* to special-case this here but it's a lot simpler if we do.
	if code == "" {
		_, existed := ns.conditions[key]
		delete(ns.conditions, key)
		return existed
	}

	updated := []condition{}
	changed := false
	for _, c := range ns.conditions[key] {
		if c.code != code {
			updated = append(updated, c)
		} else {
			changed = true
		}
	}
	if len(updated) == 0 {
		delete(ns.conditions, key)
	} else {
		ns.conditions[key] = updated
	}

	return changed
}

// ClearAllConditions clears all conditions of a given code from this namespace across all keys. It
// should only be called by the code that also *sets* the condition.
//
// It returns true if it made any changes, false otherwise.
func (ns *Namespace) ClearAllConditions(code api.Code) bool {
	changed := false
	for k, _ := range ns.conditions {
		if ns.ClearConditions(k, code) {
			changed = true
		}
	}

	return changed
}

// GetCondition gets a condition list from a key string. It returns nil, if the key doesn't exist.
func (ns *Namespace) GetCondition(key string) []condition {
	c, _ := ns.conditions[key]
	return c
}

// SetCondition adds a condition into the list of conditions for key string, returning
// true if it does not exist previously. The key must either be the constant `forest.Local`, a
// namespace name, or a string of the form "group/version/kind/namespace/name".
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

type objectsByMsg map[string][]api.AffectedObject
type objectsByCodeAndMsg map[api.Code]objectsByMsg

// convertConditions converts string -> condition{code, msg} map into condition{code, msg} -> affected map.
func (ns *Namespace) convertConditions(log logr.Logger) objectsByCodeAndMsg {
	converted := objectsByCodeAndMsg{}
	for key, conditions := range ns.conditions {
		for _, condition := range conditions {
			if _, exists := converted[condition.code]; !exists {
				converted[condition.code] = objectsByMsg{}
			}
			affectedObject := getAffectedObject(key, log)
			converted[condition.code][condition.msg] = append(converted[condition.code][condition.msg], affectedObject)
		}
	}
	return converted
}

// flatten flattens condition{code, msg} -> affected map into a list of condition{code, msg, []affected}.
func flatten(m objectsByCodeAndMsg) []api.Condition {
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

// getAffectedObject gets AffectedObject from a namespace or a string of the form
// group/version/kind/namespace/name.
func getAffectedObject(gvknn string, log logr.Logger) api.AffectedObject {
	if gvknn == Local {
		return api.AffectedObject{}
	}

	a := strings.Split(gvknn, "/")
	// The affected is a namespace.
	if len(a) == 1 {
		return api.AffectedObject{Version: "v1", Kind: "Namespace", Name: a[0]}
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
