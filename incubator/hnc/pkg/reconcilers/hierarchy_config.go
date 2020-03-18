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

package reconcilers

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
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
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/metadata"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/stats"
)

// HierarchyConfigReconciler is responsible for determining the forest structure from the Hierarchy CRs,
// as well as ensuring all objects in the forest are propagated correctly when the hierarchy
// changes. It can also set the status of the Hierarchy CRs, as well as (in rare cases) override
// part of its spec (i.e., if a parent namespace no longer exists).
type HierarchyConfigReconciler struct {
	client.Client
	Log logr.Logger

	// Forest is the in-memory data structure that is shared with all other reconcilers.
	// HierarchyConfigReconciler is responsible for keeping it up-to-date, but the other reconcilers
	// use it to determine how to propagate objects.
	Forest *forest.Forest

	// Affected is a channel of event.GenericEvent (see "Watching Channels" in
	// https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html) that is used to
	// enqueue additional namespaces that need updating.
	Affected chan event.GenericEvent

	// reconcileID is used purely to set the "rid" field in the log, so we can tell which log messages
	// were part of the same reconciliation attempt, even if multiple are running parallel (or it's
	// simply hard to tell when one ends and another begins).
	reconcileID int32

	// This is a temporary field to toggle different behaviours of this HierarchyConfigurationReconciler
	// depending on if the HierarchicalNamespaceReconciler is enabled or not. It will be removed after
	// the GitHub issue "Implement self-service namespace" is resolved
	// (https://github.com/kubernetes-sigs/multi-tenancy/issues/457)
	HNSReconcilerEnabled bool

	hnsr *HierarchicalNamespaceReconciler
}

// +kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;update;patch

// Reconcile sets up some basic variables and then calls the business logic.
func (r *HierarchyConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	if EX[req.Namespace] {
		return ctrl.Result{}, nil
	}

	stats.StartHierConfigReconcile()
	defer stats.StopHierConfigReconcile()

	ctx := context.Background()
	ns := req.NamespacedName.Namespace

	rid := (int)(atomic.AddInt32(&r.reconcileID, 1))
	log := r.Log.WithValues("ns", ns, "rid", rid)

	// TODO remove this log and use the HNSReconcilerEnabled to toggle the behavour of this
	//  reconciler accordingly. See issue: https://github.com/kubernetes-sigs/multi-tenancy/issues/467
	// Output a log for testing.
	log.Info("HC will be reconciled with", "HNSReconcilerEnabled", r.HNSReconcilerEnabled)

	return ctrl.Result{}, r.reconcile(ctx, log, ns)
}

func (r *HierarchyConfigReconciler) reconcile(ctx context.Context, log logr.Logger, nm string) error {
	// Get the singleton and namespace.
	inst, nsInst, err := r.GetInstances(ctx, log, nm)
	if err != nil {
		return err
	}

	origHC := inst.DeepCopy()
	origNS := nsInst.DeepCopy()

	update := true
	if r.HNSReconcilerEnabled {
		deleting, err := r.syncDeleting(ctx, log, inst, nsInst)
		if err != nil {
			return err
		}
		// If the namespace or the HC instance is being deleted, early exit.
		if deleting {
			return nil
		}
	} else {
		// If either object exists but is being deleted, we won't update them when we're finished syncing
		// (we should sync our internal data structure anyway just in case something's changed).  I'm not
		// sure if this is the right thing to do but the kubebuilder boilerplate included this case
		// explicitly.
		if !inst.GetDeletionTimestamp().IsZero() || !nsInst.GetDeletionTimestamp().IsZero() {
			log.Info("Singleton or namespace are being deleted; will not update")
			update = false
		}
	}

	// Sync the Hierarchy singleton with the in-memory forest.
	r.syncWithForest(log, nsInst, inst)

	// Early exit if we don't need to write anything back.
	if !update {
		return nil
	}

	// Write back if anything's changed. Early-exit if we just write back exactly what we had.
	if updated, err := r.writeInstances(ctx, log, origHC, inst, origNS, nsInst); !updated || err != nil {
		return err
	}

	// Update all the objects in this namespace. We have to do this at least *after* the tree is
	// updated, because if we don't, we could incorrectly think we've propagated the wrong objects
	// from our ancestors, or are propagating the wrong objects to our descendants.
	//
	// NB: if writeInstance didn't actually write anything - that is, if the hierarchy didn't change -
	// this update is skipped. Otherwise, we can get into infinite loops because both objects and
	// hierarchy reconcilers are enqueuing too freely. TODO: only call updateObjects when we make the
	// *kind* of changes that *should* cause objects to be updated (eg add/remove critical conditions,
	// change subtree parents, etc).
	return r.updateObjects(ctx, log, nm)
}

// syncDeleting returns true if the namespace or the HC instance is being deleted.
func (r *HierarchyConfigReconciler) syncDeleting(ctx context.Context, log logr.Logger, inst *api.HierarchyConfiguration, nsInst *corev1.Namespace) (bool, error) {
	r.Forest.Lock()
	ns := r.Forest.Get(inst.Namespace)

	switch {
	// Nothing is deleted. Early exit to do the rest of the reconciliation.
	case !ns.Deleting() && inst.DeletionTimestamp.IsZero():
		r.Forest.Unlock()
		return false, nil
	// Only deleting the singleton, no others. Remove the finalizers and early
	// exit to let it proceed.
	case !ns.Deleting() && nsInst.DeletionTimestamp.IsZero():
		r.Forest.Unlock()
		if err := r.removeFinalizers(ctx, log, inst, "only the singleton is being deleted, not the namespace"); err != nil {
			return false, err
		}
		return true, nil
	// The namespace is being deleted. Set its deleting status in the forest and
	// wait for any existing HNS instances to be gone.
	case !ns.Deleting() && !nsInst.DeletionTimestamp.IsZero():
		ns.SetDeleting()
		r.Forest.Unlock()
	// The namespace is purged by apiserver. Do nothing but unset its existence in
	// the forest.
	case ns.Deleting() && nsInst.Name == "":
		ns.UnsetExists()
		r.Forest.Unlock()
		return true, nil
	// If the namespace has deleting status in the forest but not yet being deleted,
	// delete the namespace.
	case ns.Deleting() && nsInst.DeletionTimestamp.IsZero():
		r.Forest.Unlock()
		r.deleteNamespace(ctx, log, nsInst)
	// The namespace is being deleted. Wait for any existing HNS instances to be gone.
	default:
		r.Forest.Unlock()
	}

	// Get the HNSes from the forest so that we don't need to keep looking until
	// the HNSes are purged by apiserver.
	if len(ns.OwnedHNS()) == 0 {
		r.Forest.Lock()
		// Unset the namespace existence here to avoid waiting for the namespace to be
		// purged by the apiserver, so that the HNS (if any) enqueued below can be
		// processed immediately and no additional enqueue is needed. This is safe
		// because the namespace doesn't have any finalizers and will be purged shortly.
		log.Info("Unsetting the existence of the namespace in the forest")
		ns.UnsetExists()
		// Clear owner to avoid re-creating this missing owned namespace we just deleted.
		ns.Owner = ""
		r.Forest.Unlock()

		if nsInst.Annotations[api.AnnotationOwner] != "" {
			r.hnsr.enqueue(log, inst.Namespace, nsInst.Annotations[api.AnnotationOwner], "updating the finalizer since an owned namespace is deleted")
		}

		if err := r.removeFinalizers(ctx, log, inst, "all the owned namespaces are deleted"); err != nil {
			return false, err
		}
	}
	return true, nil
}

// syncWithForest synchronizes the in-memory forest with the (in-memory) Hierarchy instance. If any
// *other* namespaces have changed, it enqueues them for later reconciliation. This method is
// guarded by the forest mutex, which means that none of the other namespaces being reconciled will
// be able to proceed until this one is finished. While the results of the reconiliation may not be
// fully written back to the apiserver yet, each namespace is reconciled in isolation (apart from
// the in-memory forest) so this is fine.
func (r *HierarchyConfigReconciler) syncWithForest(log logr.Logger, nsInst *corev1.Namespace, inst *api.HierarchyConfiguration) {
	r.Forest.Lock()
	defer r.Forest.Unlock()
	ns := r.Forest.Get(inst.ObjectMeta.Namespace)

	// Handle missing namespaces. It could be created if it's been requested as a subnamespace.
	if r.onMissingNamespace(log, ns, nsInst) {
		return
	}

	// Clear locally-set conditions in the forest so we can set them to the latest.
	hadCrit := ns.HasLocalCritCondition()
	ns.ClearConditions(forest.Local, "")

	r.markExisting(log, ns)

	r.syncOwner(log, inst, ns)
	r.syncParent(log, inst, ns)

	// Update the list of actual children, then resolve it versus the list of required children.
	r.syncChildren(log, inst, ns)

	r.syncLabel(log, nsInst, ns)

	ns.UpdateAllowCascadingDelete(inst.Spec.AllowCascadingDelete)

	// Sync all conditions. This should be placed at the end after all conditions are updated.
	r.syncConditions(log, inst, ns, hadCrit)
}

func (r *HierarchyConfigReconciler) onMissingNamespace(log logr.Logger, ns *forest.Namespace, nsInst *corev1.Namespace) bool {
	if nsInst.Name != "" {
		return false
	}

	if !ns.Exists() {
		// The namespace doesn't exist on the server, but its owner expects it to be there. Initialize
		// it so it gets created; once it is, it will be reconciled again.
		if ns.Owner != "" {
			log.Info("Will create missing namespace", "forOwner", ns.Owner)
			nsInst.Name = ns.Name()
			// Set "api.AnnotationOwner" annotation to the non-existent yet namespace.
			metadata.SetAnnotation(nsInst, api.AnnotationOwner, ns.Owner)
		}
		return true
	}

	// Remove it from the forest and notify its relatives
	r.enqueueAffected(log, "relative of deleted namespace", ns.RelativesNames()...)
	// Enqueue the HNS if the owned namespace is deleted.
	if r.HNSReconcilerEnabled {
		if ns.Owner != "" {
			r.hnsr.enqueue(log, ns.Name(), ns.Owner, "hns for the deleted owned namespace")
		}
	}
	ns.UnsetExists()
	log.Info("Removed namespace")
	return true
}

// markExisting marks the namespace as existing. If this is the first time we're reconciling this namespace,
// mark all possible relatives as being affected since they may have been waiting for this namespace.
func (r *HierarchyConfigReconciler) markExisting(log logr.Logger, ns *forest.Namespace) {
	if ns.SetExists() {
		log.Info("Reconciling new namespace")
		r.enqueueAffected(log, "relative of newly synced/created namespace", ns.RelativesNames()...)
		if ns.Owner != "" {
			r.enqueueAffected(log, "owner of the newly synced/created namespace", ns.Owner)
		}
	}
}

// syncOwner propagates the required child value from the forest to the spec if possible
// (the spec itself will be synced next), or removes the owner value if there's a problem
// and notifies the would-be parent namespace.
func (r *HierarchyConfigReconciler) syncOwner(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace) {
	if ns.Owner == "" {
		return
	}

	switch inst.Spec.Parent {
	case "":
		log.Info("Owned namespace: initializing", "owner", ns.Owner)
		inst.Spec.Parent = ns.Owner
	case ns.Owner:
		// ok
		if r.HNSReconcilerEnabled {
			// Enqueue HNS to update the state to "Ok".
			r.hnsr.enqueue(log, ns.Name(), ns.Owner, "the HNS state should be updated to ok")
		}
	default:
		if r.HNSReconcilerEnabled {
			// Enqueue the HNS to report the conflict in hierarchicalnamespace.Status.State and enqueue the
			// owner namespace to report the "SubnamespaceConflict" condition.
			log.Info("Owned namespace: conflict with parent", "owner", ns.Owner, "parent", inst.Spec.Parent)
			r.hnsr.enqueue(log, ns.Name(), ns.Owner, "owned namespace has a parent but it's not the owner")
			r.enqueueAffected(log, "owned namespace already has a parent", ns.Owner)
		} else {
			// This should never happen unless there's some crazy race condition.
			log.Info("Owned namespace: conflict with existing parent", "owner", ns.Owner, "actualParent", inst.Spec.Parent)
			r.enqueueAffected(log, "owned namespace already has a parent", ns.Owner)
			ns.Owner = "" // get back in sync with apiserver
		}
	}

	// TODO(https://github.com/kubernetes-sigs/multi-tenancy/issues/316): prevent a namespace from
	// stealing this after it's "released" from another parent.
}

func (r *HierarchyConfigReconciler) syncParent(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace) {
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
			return
		}

		// Only call other parts of the hierarchy recursively if this one was successfully updated;
		// otherwise, if you get a cycle, this could get into an infinite loop.
		if oldParent != nil {
			r.enqueueAffected(log, "removed as parent", oldParent.Name())
		}
		if curParent != nil {
			r.enqueueAffected(log, "set as parent", curParent.Name())
		}

		// Also update all descendants so they can update their labels (and in future, annotations) if
		// necessary.
		r.enqueueAffected(log, "subtree parent has changed", ns.DescendantNames()...)
	}
}

func (r *HierarchyConfigReconciler) syncLabel(log logr.Logger, nsInst *corev1.Namespace, ns *forest.Namespace) {
	// Depth label only makes sense if there's no error condition.
	if ns.HasCritCondition() {
		return
	}

	// Pre-define label depth suffix
	labelDepthSuffix := fmt.Sprintf(".tree.%s/depth", api.MetaGroup)

	// Remove all existing depth labels.
	for k := range nsInst.Labels {
		if strings.HasSuffix(k, labelDepthSuffix) {
			delete(nsInst.Labels, k)
		}
	}

	// AncestryNames includes the namespace itself.
	ancestors := ns.AncestryNames(nil)
	for i, ancestor := range ancestors {
		l := ancestor + labelDepthSuffix
		dist := strconv.Itoa(len(ancestors) - i - 1)
		metadata.SetLabel(nsInst, l, dist)
	}
}

// syncChildren looks at the current list of children and compares it to the children that
// have been marked as required. If any required children are missing, we add them to the in-memory
// forest and enqueue the (missing) child for reconciliation; we also handle various error cases.
func (r *HierarchyConfigReconciler) syncChildren(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace) {
	// Update the most recent list of children from the forest
	inst.Status.Children = ns.ChildNames()

	// Currently when HNS reconciler is not enabled, the "SubnamespaceConflict" condition is cleared and
	// updated when a parent namespace syncChildren().
	// TODO report the "SubnamespaceConflict" in the HNS reconciliation. See issue:
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/490
	if !r.HNSReconcilerEnabled {
		// We'll reset any of these conditions if they occur.
		ns.ClearAllConditions(api.SubnamespaceConflict)
	}

	// Make a set to make it easy to look up if a child is required or not
	isRequired := map[string]bool{}
	rl := ns.OwnedNames()
	// TODO Remove the spec.requiredChildren field when the hns reconciler is in use.
	//  See issue: https://github.com/kubernetes-sigs/multi-tenancy/issues/457
	// TODO Sync the hns instances in HNS reconciler, instead of syncing the spec.requiredChildren
	//  here. Update hns states in hns reconciliation. See issue:
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/487
	// Use the old spec.requiredChildren field to get the list of the self-serve subnamespaces.
	if !r.HNSReconcilerEnabled {
		rl = inst.Spec.RequiredChildren
	}
	for _, r := range rl {
		isRequired[r] = true
	}

	// Check the list of actual children against the required children
	for _, cn := range inst.Status.Children {
		cns := r.Forest.Get(cn)

		if cns.Owner != "" && cns.Owner != ns.Name() {
			// Since the list of children of this namespace came from the forest, this implies that the
			// in-memory forest is out of sync with itself: owner != parent. Obviously, this
			// should never happen.
			//
			// Let's just log an error and enqueue the other namespace so it can report the condition.
			// The forest will be reset to the correct value, below.
			log.Error(forest.OutOfSync, "While syncing children", "child", cn, "owner", cns.Owner)
			r.enqueueAffected(log, "forest out-of-sync: owner != parent", cns.Owner)
			if r.HNSReconcilerEnabled {
				r.hnsr.enqueue(log, cn, cns.Owner, "forest out-of-sync: owner != parent")
			}
		}

		if r.HNSReconcilerEnabled {
			if isRequired[cn] {
				delete(isRequired, cn)
			}
		} else {
			if isRequired[cn] {
				// This child is actually owned. Remove it from the set so we know we found it. Also, the
				// forest is almost certainly already in sync, but just set it again in case something went
				// wrong (eg, the error shown above).
				delete(isRequired, cn)
				cns.Owner = ns.Name()
			} else {
				// This isn't an owned child, but it looks like it *used* to be an owned child of this
				// namespace. Clear the Owner field from the forest to bring our state in line with
				// what's on the apiserver.
				cns.Owner = ""
			}
		}
	}

	if r.HNSReconcilerEnabled {
		for cn := range isRequired {
			r.hnsr.enqueue(log, cn, ns.Name(), "parent of the owned namespace is not the owner")
		}
	} else {
		// Anything that's still in isRequired at this point is a required child according to our own
		// spec, but it doesn't exist as a child of this namespace. There could be one of two reasons:
		// either the namespace hasn't been created (yet), in which case we just need to enqueue it for
		// reconciliation, or else it *also* is claimed by another namespace.
		for cn := range isRequired {
			log.Info("Required child is missing", "child", cn)
			cns := r.Forest.Get(cn)

			// We'll always set a condition just in case the required namespace can't be created/configured,
			// but if all is working well, the condition will be resolved as soon as the child namespace is
			// configured correctly (see the call to ClearAllConditions, above, in this function). This is
			// the default message, we'll override it if the namespace doesn't exist.
			msg := fmt.Sprintf("required subnamespace %s exists but cannot be set as a child of this namespace", cn)

			// If this child isn't claimed by another parent, claim it and make sure it gets reconciled
			// (which is when it will be created).
			if cns.Parent() == nil && (cns.Owner == "" || cns.Owner == ns.Name()) {
				cns.Owner = ns.Name()
				r.enqueueAffected(log, "required child is missing", cn)
				if !cns.Exists() {
					msg = fmt.Sprintf("required subnamespace %s does not exist", cn)
				}
			} else {
				// Someone else got it first. This should never happen if the validator is working correctly.
				other := cns.Owner
				if other == "" {
					other = cns.Parent().Name()
				}
				log.Info("Required child is already owned/claimed by another parent", "child", cn, "otherParent", other)
			}

			// Set the condition that the required child isn't an actual child. As mentioned above, if we
			// just need to create it, this condition will be removed shortly.
			ns.SetCondition(cn, api.SubnamespaceConflict, msg)
		}
	}
}

func (r *HierarchyConfigReconciler) syncConditions(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace, hadCrit bool) {
	// Sync critical conditions after all locally-set conditions are updated.
	r.syncCritConditions(log, ns, hadCrit)

	// Convert and pass in-memory conditions to HierarchyConfiguration object.
	inst.Status.Conditions = ns.Conditions(log)
	setCritAncestorCondition(log, inst, ns)
}

// syncCritConditions enqueues the children of a namespace if the existing critical conditions in the
// namespace are gone or critical conditions are newly found.
func (r *HierarchyConfigReconciler) syncCritConditions(log logr.Logger, ns *forest.Namespace, hadCrit bool) {
	hasCrit := ns.HasLocalCritCondition()

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
		if !ans.HasLocalCritCondition() {
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
func (r *HierarchyConfigReconciler) enqueueAffected(log logr.Logger, reason string, affected ...string) {
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

// removeFinalizers will update the singleton with no finalizers. It will return
// error if the singleton doesn't exist any more. This function is always put at
// the end of the early exit of a deleting case.
func (r *HierarchyConfigReconciler) removeFinalizers(ctx context.Context, log logr.Logger, inst *api.HierarchyConfiguration, reason string) error {
	log.Info("Removing the finalizers", "reason", reason)
	inst.ObjectMeta.Finalizers = nil

	stats.WriteHierConfig()
	log.Info("Updating singleton on apiserver")
	if err := r.Update(ctx, inst); err != nil {
		log.Error(err, "while updating apiserver")
		return err
	}
	return nil
}

func (r *HierarchyConfigReconciler) deleteNamespace(ctx context.Context, log logr.Logger, inst *corev1.Namespace) error {
	log.Info("Deleting namespace on apiserver")
	if err := r.Delete(ctx, inst); err != nil {
		log.Error(err, "while deleting on apiserver")
		return err
	}
	return nil
}

func (r *HierarchyConfigReconciler) GetInstances(ctx context.Context, log logr.Logger, nm string) (inst *api.HierarchyConfiguration, ns *corev1.Namespace, err error) {
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

func (r *HierarchyConfigReconciler) writeInstances(ctx context.Context, log logr.Logger, oldHC, newHC *api.HierarchyConfiguration, oldNS, newNS *corev1.Namespace) (bool, error) {
	ret := false
	if updated, err := r.writeHierarchy(ctx, log, oldHC, newHC); err != nil {
		return false, err
	} else {
		ret = ret || updated
	}

	if updated, err := r.writeNamespace(ctx, log, oldNS, newNS); err != nil {
		return false, err
	} else {
		ret = ret || updated
	}
	return ret, nil
}

func (r *HierarchyConfigReconciler) writeHierarchy(ctx context.Context, log logr.Logger, orig, inst *api.HierarchyConfiguration) (bool, error) {
	if reflect.DeepEqual(orig, inst) {
		return false, nil
	}

	stats.WriteHierConfig()
	// Make sure finalizers are set before writing the instance.
	if r.HNSReconcilerEnabled {
		inst.ObjectMeta.Finalizers = []string{api.MetaGroup}
	}
	if inst.CreationTimestamp.IsZero() {
		log.Info("Creating singleton on apiserver")
		if err := r.Create(ctx, inst); err != nil {
			log.Error(err, "while creating on apiserver")
			return false, err
		}
	} else {
		log.Info("Updating singleton on apiserver", "finalizer", inst.Finalizers, "deletionTS", inst.DeletionTimestamp)
		if err := r.Update(ctx, inst); err != nil {
			log.Error(err, "while updating apiserver")
			return false, err
		}
	}

	return true, nil
}

func (r *HierarchyConfigReconciler) writeNamespace(ctx context.Context, log logr.Logger, orig, inst *corev1.Namespace) (bool, error) {
	if reflect.DeepEqual(orig, inst) {
		return false, nil
	}

	stats.WriteNamespace()
	if inst.CreationTimestamp.IsZero() {
		log.Info("Creating namespace on apiserver")
		if err := r.Create(ctx, inst); err != nil {
			log.Error(err, "while creating on apiserver")
			return false, err
		}
	} else {
		log.Info("Updating namespace on apiserver")
		if err := r.Update(ctx, inst); err != nil {
			log.Error(err, "while updating apiserver")
			return false, err
		}
	}

	return true, nil
}

// updateObjects calls all type reconcillers in this namespace.
func (r *HierarchyConfigReconciler) updateObjects(ctx context.Context, log logr.Logger, ns string) error {
	// Use mutex to guard the read from the types list of the forest to prevent the ConfigReconciler
	// from modifying the list at the same time.
	r.Forest.Lock()
	trs := r.Forest.GetTypeSyncers()
	r.Forest.Unlock()
	for _, tr := range trs {
		if err := tr.SyncNamespace(ctx, log, ns); err != nil {
			return err
		}
	}

	return nil
}

// getSingleton returns the singleton if it exists, or creates an empty one if it doesn't.
func (r *HierarchyConfigReconciler) getSingleton(ctx context.Context, nm string) (*api.HierarchyConfiguration, error) {
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
func (r *HierarchyConfigReconciler) getNamespace(ctx context.Context, nm string) (*corev1.Namespace, error) {
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

func (r *HierarchyConfigReconciler) SetupWithManager(mgr ctrl.Manager, maxReconciles int) error {
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
