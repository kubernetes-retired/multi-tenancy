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

// NumObjectsSyncer syncs the number of propagated and source objects. ConfigReconciler implements the
// interface so that it can be called by an ObjectReconciler if the number of propagated or source objects is changed.
type NumObjectsSyncer interface {
	SyncNumObjects(logr.Logger)
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
	ObjectsStatusSyncer NumObjectsSyncer
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
		// Useful in cases where "no parent" is represented by an empty string, e.g. in the HC's
		// .spec.parent field.
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
	names := []string{}
	for nm := range f.namespaces {
		names = append(names, nm)
	}
	return names
}

type namedNamespaces map[string]*Namespace

// While storing the V in GVK is not strictly necessary to match what's in the HNC type configuration,
// as a client of the API server, HNC will be to be reading and writing versions of the API to communicate
// with the API server. Since we need the V to work with the API server anyways anyways, we will choose to
// use the GVK as the key in this map.
type objects map[schema.GroupVersionKind]map[string]*unstructured.Unstructured

// conditions stores the conditions for a single namespace, in the form obj -> code -> msg. Note
// that only one message can be stored per obj and code.
type conditions map[api.AffectedObject]map[api.Code]string

// Namespace represents a namespace in a forest. Other than its structure, it contains some
// properties useful to the reconcilers.
type Namespace struct {
	forest               *Forest
	name                 string
	parent               *Namespace
	children             namedNamespaces
	exists               bool
	allowCascadingDelete bool

	// originalObjects store the objects created by users, identified by GVK and name.
	// It serves as the source of truth for object controllers to propagate objects.
	originalObjects objects

	// conditions store conditions so that object propagation can be disabled if there's a problem
	// on this namespace.
	conditions conditions

	// IsSub indicates that this namespace is being or was created solely to live as a
	// subnamespace of the specified parent.
	IsSub bool

	// Anchors store a list of anchors in the namespace.
	Anchors []string
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

// UpdateAllowCascadingDelete updates if this namespace allows cascading deletion.
func (ns *Namespace) UpdateAllowCascadingDelete(acd bool) {
	ns.allowCascadingDelete = acd
}

// AllowsCascadingDelete returns if the namespace's or any of the owner ancestors'
// allowCascadingDelete field is set to true.
func (ns *Namespace) AllowsCascadingDelete() bool {
	if ns.allowCascadingDelete == true {
		return true
	}
	if !ns.IsSub {
		return false
	}

	// This is a subnamespace so it must have a non-nil parent. If the parent is missing, it will
	// return the default false.
	//
	// Subnamespaces can never be involved in cycles, since those can only occur at the "top" of a
	// tree and subnamespaces cannot be roots by definition. So this line can't cause a stack
	// overflow.
	return ns.parent.AllowsCascadingDelete()
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

// SetAnchors updates the anchors and returns a difference between the new/old list.
func (ns *Namespace) SetAnchors(anchors []string) (diff []string) {
	add := make(map[string]bool)
	for _, nm := range anchors {
		add[nm] = true
	}
	for _, nm := range ns.Anchors {
		if add[nm] {
			delete(add, nm)
		} else {
			// This old anchor is not in the new anchor list.
			diff = append(diff, nm)
		}
	}

	for nm, _ := range add {
		// This new anchor is not in the old anchor list.
		diff = append(diff, nm)
	}

	ns.Anchors = anchors
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

// GetOriginalObject gets an original object by name. It returns nil, if the object doesn't exist.
func (ns *Namespace) GetOriginalObject(gvk schema.GroupVersionKind, nm string) *unstructured.Unstructured {
	return ns.originalObjects[gvk][nm]
}

// HasOriginalObject returns if the namespace has an original object.
func (ns *Namespace) HasOriginalObject(gvk schema.GroupVersionKind, oo string) bool {
	return ns.GetOriginalObject(gvk, oo) != nil
}

// DeleteOriginalObject deletes an original object by name.
func (ns *Namespace) DeleteOriginalObject(gvk schema.GroupVersionKind, nm string) {
	delete(ns.originalObjects[gvk], nm)
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

// GetNumOriginalObjects returns the total number of original objects of a specific GVK in the namespace.
func (ns *Namespace) GetNumOriginalObjects(gvk schema.GroupVersionKind) int {
	return len(ns.originalObjects[gvk])
}

// GetPropagatedObjects returns all original copies in the ancestors.
func (ns *Namespace) GetPropagatedObjects(gvk schema.GroupVersionKind) []*unstructured.Unstructured {
	o := []*unstructured.Unstructured{}
	ans := ns.AncestryNames()
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

// HasLocalCritCondition returns if the namespace itself has any local critical conditions, ignoring
// its ancestors. Any code with the "Crit" prefix is a critical condition.
func (ns *Namespace) HasLocalCritCondition() bool {
	for code, _ := range ns.conditions[api.AffectedObject{}] {
		codeNm := (string)(code)
		if strings.HasPrefix(codeNm, "Crit") {
			return true
		}
	}
	return false
}

// GetCritAncestor returns the name of the first ancestor with a critical condition, or the empty
// string if there are no such ancestors. It *can* return the name of the current namespace.
func (ns *Namespace) GetCritAncestor() string {
	if ns.HasLocalCritCondition() {
		return ns.name
	}
	if ns.Parent() == nil {
		return ""
	}
	return ns.Parent().GetCritAncestor()
}

// HasCondition returns true if there's a condition with the given object and code. If code is the
// empty string, it returns true if there's _any_ condition for the given object.
func (ns *Namespace) HasCondition(obj api.AffectedObject, code api.Code) bool {
	if _, exists := ns.conditions[obj]; !exists {
		// Nothing for this obj
		return false
	}
	if code == "" {
		// Something exists for this obj; we don't care what
		return true
	}
	_, exists := ns.conditions[obj][code]
	return exists
}

// ClearCondition clears conditions in the namespace for a single object. If `code` is non-empty, it
// only clears conditions with that code, otherwise it clears all conditions for that object. It
// should only be called by the code that also *sets* the conditions.
//
// It returns true if it made any changes, false otherwise.
func (ns *Namespace) ClearCondition(obj api.AffectedObject, code api.Code) bool {
	if !ns.HasCondition(obj, code) {
		return false
	}

	if code == "" {
		delete(ns.conditions, obj)
	} else {
		delete(ns.conditions[obj], code)
	}

	return true
}

// ClearLocalConditions clears the condition(s) on this namespace.
func (ns *Namespace) ClearLocalConditions() bool {
	return ns.ClearCondition(api.AffectedObject{}, "")
}

func (ns *Namespace) ClearObsoleteConditions(log logr.Logger) {
	// Load ancestors to check CCCAncestors
	isAnc := map[string]bool{}
	for _, anc := range ns.AncestryNames() {
		// The definition of CCCAncestor doesn't include the namespace itself
		if anc != ns.name {
			isAnc[anc] = true
		}
	}

	// Load the subtree to check CCCSubtree, including the namespace itself.
	isSubtree := map[string]bool{ns.name: true}
	for _, dsc := range ns.DescendantNames() {
		isSubtree[dsc] = true
	}

	// For each affected object, remove its condition if that object is no longer relevant.
	for obj, codes := range ns.conditions {
		for code, _ := range codes {
			switch api.ClearConditionCriteria[code] {
			case api.CCCManual:
				// nop - cleared manually
			case api.CCCAncestor:
				if !isAnc[obj.Namespace] {
					log.Info("Cleared obsolete condition from old ancestor", "obj", obj, "code", code)
					ns.ClearCondition(obj, code)
				}
			case api.CCCSubtree:
				if !isSubtree[obj.Namespace] {
					log.Info("Cleared obsolete condition from old descendant", "obj", obj, "code", code)
					ns.ClearCondition(obj, code)
				}
			default:
				err := errors.New("no ClearConditionCriterion")
				log.Error(err, "In clearObsoleteConditions", "code", code, "obj", obj)
			}
		}
	}
}

// SetCondition sets a condition for the specified object and code, returning true if it does not
// exist previously or if the message has changed.
//
// Returns true if the condition wasn't previously set
func (ns *Namespace) SetCondition(obj api.AffectedObject, code api.Code, msg string) bool {
	changed := false
	if _, existed := ns.conditions[obj]; !existed {
		changed = true
		ns.conditions[obj] = map[api.Code]string{}
	}

	if oldMsg, existed := ns.conditions[obj][code]; !existed || msg != oldMsg {
		changed = true
		ns.conditions[obj][code] = msg
	}

	return changed
}

// SetLocalCondition sets a condition that applies to the current namespace.
func (ns *Namespace) SetLocalCondition(code api.Code, msg string) bool {
	return ns.SetCondition(api.AffectedObject{}, code, msg)
}

// Conditions returns a list of conditions in the namespace in the format expected by the API.
func (ns *Namespace) Conditions() []api.Condition {
	// Treat the code/msg combination as a combined key.
	type codeMsg struct {
		code api.Code
		msg  string
	}

	// Reorder so that the objects are grouped by code and message
	byCM := map[codeMsg][]api.AffectedObject{}
	for obj, codes := range ns.conditions {
		for code, msg := range codes {
			cm := codeMsg{code: code, msg: msg}
			byCM[cm] = append(byCM[cm], obj)
		}
	}

	// Flatten into a list of conditions
	conds := []api.Condition{}
	for cm, objs := range byCM {
		// If the only affected object is unnamed (e.g., it refers to the current namespace), omit it.
		c := api.Condition{Code: cm.code, Msg: cm.msg}
		if len(objs) > 0 || objs[0].Name != "" {
			api.SortAffectedObjects(objs)
			c.Affects = objs
		}
		conds = append(conds, c)
	}

	sort.Slice(conds, func(i, j int) bool {
		if conds[i].Code != conds[j].Code {
			return conds[i].Code < conds[j].Code
		}
		return conds[i].Msg < conds[j].Msg
	})

	if len(conds) == 0 {
		conds = nil // prevent anything from appearing in the status
	}
	return conds
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
