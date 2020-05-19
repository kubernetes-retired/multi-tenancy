package forest

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
