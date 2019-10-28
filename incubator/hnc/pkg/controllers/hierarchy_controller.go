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
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
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
	// Get the singleton and namespace.
	inst, nsInst, err := r.getInstances(ctx, log, nm)
	if err != nil {
		return err
	}

	origHC := inst.DeepCopy()
	origNS := nsInst.DeepCopy()

	// If either object exists but is being deleted, we won't update them when we're finished syncing
	// (we should sync our internal data structure anyway just in case something's changed).  I'm not
	// sure if this is the right thing to do but the kubebuilder boilerplate included this case
	// explicitly.
	update := true
	if !inst.GetDeletionTimestamp().IsZero() || !nsInst.GetDeletionTimestamp().IsZero() {
		log.Info("Singleton or namespace are being deleted; will not update")
		update = false
	}

	// Sync the Hierarchy singleton with the in-memory forest.
	r.syncWithForest(log, nsInst, inst)

	// Early exit if we don't need to write anything back.
	if !update {
		return nil
	}

	// Write back if anything's changed.
	if err := r.writeInstances(ctx, log, origHC, inst, origNS, nsInst); err != nil {
		return err
	}

	// Update all the objects in this namespace. We have to do this at least *after* the tree is
	// updated, because if we don't, we could incorrectly think we've propagated the wrong objects
	// from our ancestors, or are propagating the wrong objects to our descendants.
	return r.updateObjects(ctx, log, nm)
}

// syncWithForest synchronizes the in-memory forest with the (in-memory) Hierarchy instance. If any
// *other* namespaces have changed, it enqueues them for later reconciliation. This method is
// guarded by the forest mutex, which means that none of the other namespaces being reconciled will
// be able to proceed until this one is finished. While the results of the reconiliation may not be
// fully written back to the apiserver yet, each namespace is reconciled in isolation (apart from
// the in-memory forest) so this is fine.
func (r *HierarchyReconciler) syncWithForest(log logr.Logger, nsInst *corev1.Namespace, inst *api.HierarchyConfiguration) {
	r.Forest.Lock()
	defer r.Forest.Unlock()
	ns := r.Forest.Get(inst.ObjectMeta.Namespace)

	// Handle missing namespaces. It could be created if it's been requested as a subnamespace.
	if r.onMissingNamespace(log, ns, nsInst) {
		return
	}

	// Clear locally-set conditions in the forest so we can set them to the latest.
	hadCrit := ns.HasCritCondition()
	ns.ClearConditions(forest.Local)

	r.markExisting(log, ns)

	r.syncRequiredChildOf(log, inst, ns)
	r.syncParent(log, inst, ns)

	// Update the list of actual children, then resolve it versus the list of required children.
	r.syncChildren(log, inst, ns)

	r.syncLabel(log, nsInst, ns)

	// Sync all conditions. This should be placed at the end after all conditions are updated.
	r.syncConditions(log, inst, ns, hadCrit)
}

func (r *HierarchyReconciler) onMissingNamespace(log logr.Logger, ns *forest.Namespace, nsInst *corev1.Namespace) bool {
	if nsInst.Name != "" {
		return false
	}

	if !ns.Exists() {
		// The namespace doesn't exist on the server, but it's been requested for a parent. Initialize
		// it so it gets created; once it is, it will be reconciled again.
		if ns.RequiredChildOf != "" {
			log.Info("Will create missing namespace", "forParent", ns.RequiredChildOf)
			nsInst.Name = ns.Name()
		}
		return true
	}

	// Remove it from the forest and notify its relatives
	r.enqueueAffected(log, "relative of deleted namespace", ns.RelativesNames()...)
	ns.UnsetExists()
	log.Info("Removed namespace")
	return true
}

// markExisting marks the namespace as existing. If this is the first time we're reconciling this namespace,
// mark all possible relatives as being affected since they may have been waiting for this namespace.
func (r *HierarchyReconciler) markExisting(log logr.Logger, ns *forest.Namespace) {
	if ns.SetExists() {
		log.Info("Reconciling new namespace")
		r.enqueueAffected(log, "relative of newly created namespace", ns.RelativesNames()...)
		if ns.RequiredChildOf != "" {
			r.enqueueAffected(log, "parent of newly created required subnamespace", ns.RequiredChildOf)
		}
	}
}

func (r *HierarchyReconciler) syncRequiredChildOf(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace) {
	if ns.RequiredChildOf == "" {
		return
	}

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
		r.enqueueAffected(log, "wrong parent set as a parent", inst.Spec.Parent)
		ns.SetCondition(forest.Local, api.CritRequiredChildConflict, msg)
	}
}

func (r *HierarchyReconciler) syncParent(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace) {
	// Sync this namespace with its current parent.
	var curParent *forest.Namespace
	if inst.Spec.Parent != "" {
		curParent = r.Forest.Get(inst.Spec.Parent)
		if !curParent.Exists() {
			log.Info("Missing parent", "parent", inst.Spec.Parent)
			ns.SetCondition(forest.Local, api.CritParentMissing, "missing parent")
		}
	}

	// Update the in-memory hierarchy if it's changed
	oldParent := ns.Parent()
	if oldParent != curParent {
		log.Info("Updating parent", "old", oldParent.Name(), "new", curParent.Name())
		if err := ns.SetParent(curParent); err != nil {
			log.Info("Couldn't update parent", "reason", err, "parent", inst.Spec.Parent)
			ns.SetCondition(forest.Local, api.CritParentInvalid, err.Error())
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
}

func (r *HierarchyReconciler) syncLabel(log logr.Logger, nsInst *corev1.Namespace, ns *forest.Namespace) {
	// Depth label only makes sense if there's no error condition.
	if ns.HasCondition() {
		return
	}

	// AncestoryNames includes the namespace itself.
	ancestors := ns.AncestoryNames(nil)
	for i, ancestor := range ancestors {
		labelDepthSuffix := ancestor + ".tree." + api.MetaGroup + "/depth"
		dist := strconv.Itoa(len(ancestors) - i - 1)
		setLabel(nsInst, labelDepthSuffix, dist)
	}

	// All namespaces in its subtree should update the labels as well.
	//
	// TODO: only enqueue these when the parent has changed? We don't need to enqueue this every time
	// the hc is updated for any reason.
	if descendants := ns.DescendantNames(); descendants != nil {
		r.enqueueAffected(log, "update depth label", descendants...)
	}
}

// syncChildren looks at the current list of children and compares it to the children that
// have been marked as required. If any required children are missing, we add them to the in-memory
// forest and enqueue the (missing) child for reconciliation; we also handle various error cases.
func (r *HierarchyReconciler) syncChildren(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace) {
	inst.Status.Children = ns.ChildNames()
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
			cns.RequiredChildOf = ns.Name() // mark in in the forest
			delete(reqSet, cn)              // remove so we know we found it
			if cns.Exists() && cns.Parent() != ns { // condition if it's assigned elsewhere
				msg := fmt.Sprintf("required subnamespace %s exists but has parent %s", cn, cns.Parent().Name())
				ns.SetCondition(forest.Local, api.CritRequiredChildConflict, msg)
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
			ns.SetCondition(forest.Local, api.CritRequiredChildConflict, msg)
		}
	}

	// Anything that's still in reqSet at this point is a required child, but it doesn't exist as a
	// child of this namespace. There could be one of two reasons: either the namespace hasn't been
	// created (yet), in which case we just need to enqueue it for reconciliation, or else it *also*
	// is claimed by another namespace.
	for cn := range reqSet {
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
			ns.SetCondition(forest.Local, api.CritRequiredChildConflict, msg)
		} else {
			// Someone else got it first. This should never happen if the validator is working correctly.
			log.Info("Required child is claimed by another parent", "child", cn, "otherParent", cns.RequiredChildOf)
			msg := fmt.Sprintf("required child namespace %s is already a child namespace of %s", cn, cns.RequiredChildOf)
			ns.SetCondition(forest.Local, api.CritRequiredChildConflict, msg)
		}
	}
}

func (r *HierarchyReconciler) syncConditions(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace, hadCrit bool) {
	// Sync critical conditions after all locally-set conditions are updated.
	r.syncCritConditions(log, ns, hadCrit)

	// Convert and pass in-memory conditions to HierarchyConfiguration object.
	inst.Status.Conditions = ns.Conditions(log)
	setCritAncestorCondition(log, inst, ns)
}

// syncCritConditions enqueues the children of a namespace if the existing critical conditions in the
// namespace are gone or critical conditions are newly found.
func (r *HierarchyReconciler) syncCritConditions(log logr.Logger, ns *forest.Namespace, hadCrit bool) {
	hasCrit := ns.HasCritCondition()

	// Early exit if there's no need to enqueue relatives.
	if hadCrit == hasCrit {
		return
	}

	msg := "added"
	if hadCrit == true {
		msg = "removed"
	}
	log.Info("Critical conditions are " + msg)
	r.enqueueAffected(log, "descendant of a namespace with critical conditions "+msg, ns.DescendantNames()...)
}

func setCritAncestorCondition(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace) {
	ans := ns.Parent()
	for ans != nil {
		if !ans.HasCritCondition() {
			ans = ans.Parent()
			continue
		}
		log.Info("Ancestor has a critical condition", "ancestor", ans.Name())
		msg := fmt.Sprintf("Propagation paused in the subtree of %s due to a critical condition", ans.Name())
		condition := api.Condition{
			Code:    api.CritAncestor,
			Msg:     msg,
			Affects: []api.AffectedObject{{Namespace: ans.Name()}},
		}
		inst.Status.Conditions = append(inst.Status.Conditions, condition)
		return
	}
}

// enqueueAffected enqueues all affected namespaces for later reconciliation. This occurs in a
// goroutine so the caller doesn't block; since the reconciler is never garbage-collected, this is
// safe.
func (r *HierarchyReconciler) enqueueAffected(log logr.Logger, reason string, affected ...string) {
	go func() {
		for _, nm := range affected {
			log.Info("Enqueuing for reconcilation", "affected", nm, "reason", reason)
			// The watch handler doesn't care about anything except the metadata.
			inst := &api.HierarchyConfiguration{}
			inst.ObjectMeta.Name = api.Singleton
			inst.ObjectMeta.Namespace = nm
			r.Affected <- event.GenericEvent{Meta: inst}
		}
	}()
}

func (r *HierarchyReconciler) getInstances(ctx context.Context, log logr.Logger, nm string) (inst *api.HierarchyConfiguration, ns *corev1.Namespace, err error) {
	inst, err = r.getSingleton(ctx, nm)
	if err != nil {
		log.Error(err, "Couldn't read singleton")
		return nil, nil, err
	}
	ns, err = r.getNamespace(ctx, nm)
	if err != nil {
		log.Error(err, "Couldn't read namespace")
		return nil, nil, err
	}

	return inst, ns, nil
}

func (r *HierarchyReconciler) writeInstances(ctx context.Context, log logr.Logger, oldHC, newHC *api.HierarchyConfiguration, oldNS, newNS *corev1.Namespace) error {
	if err := r.writeHierarchy(ctx, log, oldHC, newHC); err != nil {
		return err
	}
	if err := r.writeNamespace(ctx, log, oldNS, newNS); err != nil {
		return err
	}
	return nil
}

func (r *HierarchyReconciler) writeHierarchy(ctx context.Context, log logr.Logger, orig, inst *api.HierarchyConfiguration) error {
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
func (r *HierarchyReconciler) getSingleton(ctx context.Context, nm string) (*api.HierarchyConfiguration, error) {
	nnm := types.NamespacedName{Namespace: nm, Name: api.Singleton}
	inst := &api.HierarchyConfiguration{}
	if err := r.Get(ctx, nnm, inst); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}

		// It doesn't exist - initialize it to a sane initial value.
		inst.ObjectMeta.Name = api.Singleton
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

func (r *HierarchyReconciler) SetupWithManager(mgr ctrl.Manager, maxReconciles int) error {
	// Maps namespaces to their singletons
	nsMapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      api.Singleton,
					Namespace: a.Meta.GetName(),
				}},
			}
		})
	opts := controller.Options{
		MaxConcurrentReconciles: maxReconciles,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.HierarchyConfiguration{}).
		Watches(&source.Channel{Source: r.Affected}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: nsMapFn}).
		WithOptions(opts).
		Complete(r)
}
