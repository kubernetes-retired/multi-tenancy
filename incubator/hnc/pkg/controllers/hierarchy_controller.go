/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// HierarchyReconciler is responsible for determining the forest structure from the Hierarchy CRs,
// as well as ensuring all objects in the forest are propagated correctly when the hierarchy
// changes. It can also set the status of the Hierarchy CRs, as well as (in rare cases) override
// part of its spec (i.e., if a parent namespace no longer exists).
type HierarchyReconciler struct {
	client.Client
	Log logr.Logger

	// Forest is the in-memory data structure that is shared with all other reconcilers.
	// HierarchyReconciler is responsible for keeping it up-to-date, but the other reconcilers
	// use it to determine how to propagate objects.
	Forest *forest.Forest

	// Types is a list of other reconcillers that HierarchyReconciler can call if the hierarchy
	// changes. This will force all objects to be re-propagated.
	//
	// This is probably wildly inefficient, and we can probably make better use of things like
	// owner references to make this better. But for a PoC, it works just fine.
	Types []NamespaceSyncer

	// Affected is a channel of event.GenericEvent (see "Watching Channels" in
	// https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html) that is used to
	// enqueue additional namespaces that need updating.
	Affected chan event.GenericEvent

	// These locks prevent more than one goroutine from attempting to reconcile any one namespace a a
	// time. Without this, the forest may stay in sync, but the changes to the apiserver could be
	// committed out of order with no guarantee that the reconciler will be called again.
	namespaceLocks sync.Map

	// reconcileID is used purely to set the "rid" field in the log, so we can tell which log messages
	// were part of the same reconciliation attempt, even if multiple are running parallel (or it's
	// simply hard to tell when one ends and another begins).
	reconcileID int32
}

// NamespaceSyncer syncs various aspects of a namespace. The HierarchyReconciler both implements
// it (so it can be called by NamespaceSyncer) and uses it (to sync the objects in the
// namespace).
type NamespaceSyncer interface {
	SyncNamespace(context.Context, logr.Logger, string) error
}

// +kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;update;patch

// Reconcile simply calls SyncNamespace, which can also be called if a namespace is created or
// deleted.
func (r *HierarchyReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	ns := req.NamespacedName.Namespace

	id := r.lockNamespace(ns)
	defer r.unlockNamespace(ns)

	log := r.Log.WithValues("ns", ns, "rid", id)
	return ctrl.Result{}, r.reconcile(ctx, log, ns)
}

func (r *HierarchyReconciler) reconcile(ctx context.Context, log logr.Logger, nm string) error {
	// Get the singleton and namespace
	inst, err := r.getSingleton(ctx, nm)
	if err != nil {
		log.Error(err, "Couldn't read singleton")
		return err
	}
	ns, err := r.getNamespace(ctx, nm)
	if err != nil {
		log.Error(err, "Couldn't read namespace")
		return err
	}

	// If either object exists but is being deleted, we won't update them when we're finished syncing
	// (we should sync our internal data structure anyway just in case something's changed).  I'm not
	// sure if this is the right thing to do but the kubebuilder boilerplate included this case
	// explicitly.
	update := true
	if !inst.GetDeletionTimestamp().IsZero() || !ns.GetDeletionTimestamp().IsZero() {
		log.Info("Singleton or namespace are being deleted; will not update")
		update = false
	}

	// Sync the tree.
	if err := r.updateTree(ctx, log, ns, inst, update); err != nil {
		return err
	}

	if update {
		// Update all the objects in this namespace. We have to do this at least *after* the tree is
		// updated, because if we don't, we could incorrectly think we've propagated the wrong objects
		// from our ancestors, or are propagating the wrong objects to our descendants.
		return r.updateObjects(ctx, log, nm)
	}

	return nil
}

// updateTree syncs the Hierarchy singleton with the in-memory forest (writing back to the apiserver
// if necessary and requested) and calls itself on any affected namespaces if the hierarchy has
// changed.
//
// TODO: store the conditions in the in-memory forest so that object propagation can be disabled if
// there's a problem on the namespace.
func (r *HierarchyReconciler) updateTree(ctx context.Context, log logr.Logger, nsInst *corev1.Namespace, inst *tenancy.Hierarchy, update bool) error {
	// Update the in-memory data structures
	origHier := inst.DeepCopy()
	origNS := nsInst.DeepCopy()
	r.syncWithForest(log, nsInst, inst)

	// Early exit if we don't need to write anything back.
	if !update {
		return nil
	}

	// Write back if anything's changed.
	if err := r.writeHierarchy(ctx, log, origHier, inst); err != nil {
		return err
	}
	if err := r.writeNamespace(ctx, log, origNS, nsInst); err != nil {
		return err
	}

	return nil
}

func (r *HierarchyReconciler) writeHierarchy(ctx context.Context, log logr.Logger, orig, inst *tenancy.Hierarchy) error {
	if reflect.DeepEqual(orig, inst) {
		return nil
	}

	if inst.CreationTimestamp.IsZero() {
		log.Info("Creating singleton on apiserver")
		if err := r.Create(ctx, inst); err != nil {
			log.Error(err, "while creating on apiserver")
			return err
		}
	} else {
		log.Info("Updating singleton on apiserver")
		if err := r.Update(ctx, inst); err != nil {
			log.Error(err, "while updating apiserver")
			return err
		}
	}

	return nil
}

func (r *HierarchyReconciler) writeNamespace(ctx context.Context, log logr.Logger, orig, inst *corev1.Namespace) error {
	if reflect.DeepEqual(orig, inst) {
		return nil
	}

	if inst.CreationTimestamp.IsZero() {
		log.Info("Creating namespace on apiserver")
		if err := r.Create(ctx, inst); err != nil {
			log.Error(err, "while creating on apiserver")
			return err
		}
	} else {
		log.Info("Updating namespace on apiserver")
		if err := r.Update(ctx, inst); err != nil {
			log.Error(err, "while updating apiserver")
			return err
		}
	}

	return nil
}

// updateObjects calls all type reconcillers in this namespace.
func (r *HierarchyReconciler) updateObjects(ctx context.Context, log logr.Logger, ns string) error {
	for _, tr := range r.Types {
		if err := tr.SyncNamespace(ctx, log, ns); err != nil {
			return err
		}
	}

	return nil
}

// syncWithForest synchronizes the in-memory forest with the (in-memory) Hierarchy instance. If any
// *other* namespaces have changed, it enqueues them for later reconciliation. This method is
// guarded by the forest mutex, which means that none of the other namespaces being reconciled will
// be able to proceed until this one is finished. While the results of the reconiliation may not be
// fully written back to the apiserver yet, each namespace is reconciled in isolation (apart from
// the in-memory forest) so this is fine.
func (r *HierarchyReconciler) syncWithForest(log logr.Logger, nsInst *corev1.Namespace, inst *tenancy.Hierarchy) {
	r.Forest.Lock()
	defer r.Forest.Unlock()
	ns := r.Forest.Get(inst.ObjectMeta.Namespace)

	// Handle missing namespaces. It could be created if it's been requested as a subnamespace.
	if nsInst.Name == "" {
		r.onMissingNamespace(log, ns, nsInst)
		return
	}

	// Mark the namespace as existing. If this is the first time we're reconciling this namespace,
	// mark all possible relatives as being affected since they may have been waiting for this
	// namespace.
	conds := []tenancy.Condition{}
	if ns.SetExists() {
		log.Info("Reconciling new namespace")
		r.enqueueAffected(log, "relative of newly created namespace", ns.RelativesNames()...)
		if ns.RequiredChildOf != "" {
			r.enqueueAffected(log, "parent of newly created required subnamespace", ns.RequiredChildOf)
		}
	}

	// If this namespace is a required child, update its status accordingly.
	if ns.RequiredChildOf != "" {
		switch {
		case inst.Spec.Parent == "":
			log.Info("Required subnamespace: initializing", "parent", ns.RequiredChildOf)
			inst.Spec.Parent = ns.RequiredChildOf
		case inst.Spec.Parent == ns.RequiredChildOf:
			// ok
		default:
			log.Info("Required subnamespace: assigned to wrong parent", "intended", ns.RequiredChildOf, "actual", inst.Spec.Parent)
			r.enqueueAffected(log, "incorrect parent of the subnamespace", inst.Spec.Parent)
			msg := fmt.Sprintf("required child of %s but parent is set to %s", ns.RequiredChildOf, inst.Spec.Parent)
			conds = append(conds, tenancy.Condition{Msg: msg})
		}
	}

	// Sync this namespace with its current parent.
	var curParent *forest.Namespace
	if inst.Spec.Parent != "" {
		curParent = r.Forest.Get(inst.Spec.Parent)
		if !curParent.Exists() {
			log.Info("Missing parent", "parent", inst.Spec.Parent)
			conds = append(conds, tenancy.Condition{Msg: "missing parent"})
		}
	}

	// Update the in-memory hierarchy if it's changed
	oldParent := ns.Parent()
	if oldParent != curParent {
		log.Info("Updating parent", "old", oldParent.Name(), "new", curParent.Name())
		if err := ns.SetParent(curParent); err != nil {
			log.Info("Couldn't update parent", "reason", err, "parent", inst.Spec.Parent)
			conds = append(conds, tenancy.Condition{Msg: err.Error()})
		} else {
			// Only call other parts of the hierarchy recursively if this one was successfully updated;
			// otherwise, if you get a cycle, this could get into an infinite loop.
			if oldParent != nil {
				r.enqueueAffected(log, "removed as parent", oldParent.Name())
			}
			if curParent != nil {
				r.enqueueAffected(log, "set as parent", curParent.Name())
			}
		}
	}

	// Update the list of actual children, then resolve it versus the list of required children.
	inst.Status.Children = ns.ChildNames()
	conds = append(conds, r.syncRequiredChildren(log, inst, ns)...)

	// Update all other changed fields. If they're empty, ensure they're nil so that they compare
	// properly.
	if len(conds) > 0 {
		inst.Status.Conditions = conds
	} else {
		inst.Status.Conditions = nil
	}
}

func (r *HierarchyReconciler) onMissingNamespace(log logr.Logger, ns *forest.Namespace, nsInst *corev1.Namespace) {
	if !ns.Exists() {
		// The namespace doesn't exist on the server, but it's been requested for a parent. Initialize
		// it so it gets created; once it is, it will be reconciled again.
		if ns.RequiredChildOf != "" {
			log.Info("Will create missing namespace", "forParent", ns.RequiredChildOf)
			nsInst.Name = ns.Name()
		}
		return
	}

	// Remove it from the forest and notify its relatives
	r.enqueueAffected(log, "relative of deleted namespace", ns.RelativesNames()...)
	ns.UnsetExists()
	log.Info("Removed namespace")
}

// enqueueAffected enqueues all affected namespaces for later reconciliation. This occurs in a
// goroutine so the caller doesn't block; since the reconciler is never garbage-collected, this is
// safe.
func (r *HierarchyReconciler) enqueueAffected(log logr.Logger, reason string, affected ...string) {
	go func() {
		for _, nm := range affected {
			log.Info("Enqueuing for reconcilation", "affected", nm, "reason", reason)
			// The watch handler doesn't care about anything except the metadata.
			inst := &tenancy.Hierarchy{}
			inst.ObjectMeta.Name = tenancy.Singleton
			inst.ObjectMeta.Namespace = nm
			r.Affected <- event.GenericEvent{Meta: inst}
		}
	}()
}

// syncRequiredChildren looks at the current list of children and compares it to the children that
// have been marked as required. If any required children are missing, we add them to the in-memory
// forest and enqueue the (missing) child for reconciliation; we also handle various error cases.
func (r *HierarchyReconciler) syncRequiredChildren(log logr.Logger, inst *tenancy.Hierarchy, ns *forest.Namespace) []tenancy.Condition {
	conds := []tenancy.Condition{}

	// Make a set to make it easy to look up if a child is required or not
	reqSet := map[string]bool{}
	for _, r := range inst.Spec.RequiredChildren {
		reqSet[r] = true
	}

	// Check the list of actual children against the required children
	for _, cn := range inst.Status.Children {
		cns := r.Forest.Get(cn)
		if _, isRequired := reqSet[cn]; isRequired {
			// This is a required child of this namespace
			cns.RequiredChildOf = ns.Name()         // mark in in the forest
			delete(reqSet, cn)                      // remove so we know we found it
			if cns.Exists() && cns.Parent() != ns { // condition if it's assigned elsewhere
				msg := fmt.Sprintf("required subnamespace %s exists but has parent %s", cn, cns.Parent().Name())
				conds = append(conds, tenancy.Condition{Msg: msg})
			}
		} else if cns.RequiredChildOf == ns.Name() {
			// This isn't a required child, but it looks like it *used* to be a required child of this
			// namespace. Clear the RequiredChildOf field from the forest to bring our state in line with
			// what's on the apiserver.
			cns.RequiredChildOf = ""
		} else if cns.RequiredChildOf != "" {
			// This appears to be the required child of *another* namespace, and yet it's currently our
			// child! Oops. Add a condition to this namespace so we know we have a child that we
			// shouldn't.
			msg := fmt.Sprintf("child namespace %s should be a child of %s", cn, cns.RequiredChildOf)
			conds = append(conds, tenancy.Condition{Msg: msg})
		}
	}

	// Anything that's still in reqSet at this point is a required child, but it doesn't exist as a
	// child of this namespace. There could be one of two reasons: either the namespace hasn't been
	// created (yet), in which case we just need to enqueue it for reconciliation, or else it *also*
	// is claimed by another namespace.
	for cn, _ := range reqSet {
		log.Info("Required child is missing", "child", cn)
		cns := r.Forest.Get(cn)
		// If this child isn't claimed by another parent, claim it and make sure it gets reconciled
		// (which is when it will be created).
		if cns.RequiredChildOf == "" || cns.RequiredChildOf == ns.Name() {
			cns.RequiredChildOf = ns.Name()
			r.enqueueAffected(log, "required child is missing", cn)
			// We expect this to be resolved shortly, but set a condition just in case it's not.
			var msg string
			if !cns.Exists() {
				msg = fmt.Sprintf("required subnamespace %s does not exist", cn)
			} else {
				msg = fmt.Sprintf("required subnamespace %s exists but cannot be set as a child of this namespace", cn)
			}
			conds = append(conds, tenancy.Condition{Msg: msg})
		} else {
			// Someone else got it first. This should never happen if the validator is working correctly.
			log.Info("Required child is claimed by another parent", "child", cn, "otherParent", cns.RequiredChildOf)
			msg := fmt.Sprintf("required child namespace %s is already a child namespace of %s", cn, cns.RequiredChildOf)
			conds = append(conds, tenancy.Condition{Msg: msg})
		}
	}

	return conds
}

// lockNamespace ensures that the controller cannot attempt to reconcile the same namespace more
// than once at a time. When it is finished, a per-namespace lock will be held, which _must_ be
// released by unlockNamespace.
//
// It also return an integral reconciliation ID, which is unsed in logs to disambiguate which
// messages came from which attempt.
func (r *HierarchyReconciler) lockNamespace(nm string) int {
	m, _ := r.namespaceLocks.LoadOrStore(nm, &sync.Mutex{})
	m.(*sync.Mutex).Lock()

	return (int)(atomic.AddInt32(&r.reconcileID, 1))
}

// unlockNamespace releases the per-namespace lock.
func (r *HierarchyReconciler) unlockNamespace(nm string) {
	m, _ := r.namespaceLocks.Load(nm)
	m.(*sync.Mutex).Unlock()
}

// getSingleton returns the singleton if it exists, or creates an empty one if it doesn't.
func (r *HierarchyReconciler) getSingleton(ctx context.Context, nm string) (*tenancy.Hierarchy, error) {
	nnm := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
	inst := &tenancy.Hierarchy{}
	if err := r.Get(ctx, nnm, inst); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}

		// It doesn't exist - initialize it to a sane initial value.
		inst.ObjectMeta.Name = tenancy.Singleton
		inst.ObjectMeta.Namespace = nm
	}

	return inst, nil
}

// getNamespace returns the namespace if it exists, or returns an invalid, blank, unnamed one if it
// doesn't. This allows it to be trivially identified as a namespace that doesn't exist, and also
// allows us to easily modify it if we want to create it.
func (r *HierarchyReconciler) getNamespace(ctx context.Context, nm string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{}
	nnm := types.NamespacedName{Name: nm}
	if err := r.Get(ctx, nnm, ns); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		return &corev1.Namespace{}, nil
	}
	return ns, nil
}

func (r *HierarchyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Maps namespaces to their singletons
	nsMapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      tenancy.Singleton,
					Namespace: a.Meta.GetName(),
				}},
			}
		})
	return ctrl.NewControllerManagedBy(mgr).
		For(&tenancy.Hierarchy{}).
		Watches(&source.Channel{Source: r.Affected}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: nsMapFn}).
		Complete(r)
}
