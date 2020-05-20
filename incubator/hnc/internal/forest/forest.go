// Package forest defines the Forest type.
package forest

import (
	"context"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
)

// Forest defines a forest of namespaces - that is, a set of trees. It includes methods to mutate
// the forest and track problems such as cycles.
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
}

type namedNamespaces map[string]*Namespace

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
