package forest

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
)

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

	// Manager stores the manager of the namespace. The default value
	// "hnc.x-k8s.io" means it's managed by HNC.
	Manager string

	// ExternalTreeLabels stores external tree labels if this namespace is an external namespace.
	// It will be empty if the namespace is not external. External namespace will at least have one
	// tree label of itself. The key is the tree label without ".tree.hnc.x-k8s.io/depth" suffix.
	// The value is the depth.
	ExternalTreeLabels map[string]int
}

// Name returns the name of the namespace, of "<none>" if the namespace is nil.
func (ns *Namespace) Name() string {
	if ns == nil {
		return "<none>"
	}
	return ns.name
}

// Parent returns a pointer to the parent namespace.
func (ns *Namespace) Parent() *Namespace {
	return ns.parent
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

// IsExternal returns true if the namespace is not managed by HNC.
func (ns *Namespace) IsExternal() bool {
	return len(ns.ExternalTreeLabels) > 0
}
