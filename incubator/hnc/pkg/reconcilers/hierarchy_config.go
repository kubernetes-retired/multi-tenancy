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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	return ctrl.Result{}, r.reconcile(ctx, log, ns)
}

func (r *HierarchyConfigReconciler) reconcile(ctx context.Context, log logr.Logger, nm string) error {
	nsInst, err := r.getNamespace(ctx, nm)
	if err != nil {
		if errors.IsNotFound(err) {
			// The namespace doesn't exist or is purged. Update the forest and exit.
			// (There must be no HC instance and we cannot create one.)
			r.onMissingNamespace(log, nm)
			return nil
		}
		return err
	}
	// Get singleton from apiserver. If it doesn't exist, initialize one.
	inst, err := r.getSingleton(ctx, nm)
	if err != nil {
		return err
	}
	// Get a list of HNS instance namespaces from apiserver.
	hnsnms, err := r.getHierarchicalNamespaceNames(ctx, nm)
	if err != nil {
		return err
	}

	origHC := inst.DeepCopy()
	origNS := nsInst.DeepCopy()

	r.updateFinalizers(ctx, log, inst, nsInst, hnsnms)

	// Sync the Hierarchy singleton with the in-memory forest.
	r.syncWithForest(log, nsInst, inst, hnsnms)

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

func (r *HierarchyConfigReconciler) onMissingNamespace(log logr.Logger, nm string) {
	r.Forest.Lock()
	defer r.Forest.Unlock()
	ns := r.Forest.Get(nm)

	if ns.Exists() {
		r.enqueueAffected(log, "relative of deleted namespace", ns.RelativesNames()...)
		ns.UnsetExists()
		log.Info("Removed namespace")
	}
}

func (r *HierarchyConfigReconciler) updateFinalizers(ctx context.Context, log logr.Logger, inst *api.HierarchyConfiguration, nsInst *corev1.Namespace, hnsnms []string) {
	switch {
	case len(hnsnms) == 0:
		// There's no owned namespaces in this namespace. The HC instance can be
		// safely deleted anytime.
		log.Info("Remove finalizers since there's no HNS instance in the namespace.")
		inst.ObjectMeta.Finalizers = nil
	case !inst.DeletionTimestamp.IsZero() && nsInst.DeletionTimestamp.IsZero():
		// If the HC instance is being deleted but not the namespace (which means
		// it's not a cascading delete), remove the finalizers to let it go through.
		// This is the only case the finalizers can be removed even when the
		// namespace has owned namespaces. (A default HC will be recreated later.)
		log.Info("Remove finalizers to allow a single deletion of the singleton (not involved in a cascading deletion).")
		inst.ObjectMeta.Finalizers = nil
	default:
		log.Info("Add finalizers since there's HNS instance(s) in the namespace.")
		inst.ObjectMeta.Finalizers = []string{api.FinalizerHasOwnedNamespace}
	}
}

// syncWithForest synchronizes the in-memory forest with the (in-memory) Hierarchy instance. If any
// *other* namespaces have changed, it enqueues them for later reconciliation. This method is
// guarded by the forest mutex, which means that none of the other namespaces being reconciled will
// be able to proceed until this one is finished. While the results of the reconiliation may not be
// fully written back to the apiserver yet, each namespace is reconciled in isolation (apart from
// the in-memory forest) so this is fine.
func (r *HierarchyConfigReconciler) syncWithForest(log logr.Logger, nsInst *corev1.Namespace, inst *api.HierarchyConfiguration, hnsnms []string) {
	r.Forest.Lock()
	defer r.Forest.Unlock()
	ns := r.Forest.Get(inst.ObjectMeta.Namespace)

	// Clear locally-set conditions in the forest so we can set them to the latest.
	hadCrit := ns.HasLocalCritCondition()
	ns.ClearLocalCondition("")

	r.syncOwner(log, inst, nsInst, ns)
	r.markExisting(log, ns)

	r.syncParent(log, inst, ns)
	inst.Status.Children = ns.ChildNames()
	r.syncHNSes(log, ns, hnsnms)
	ns.UpdateAllowCascadingDelete(inst.Spec.AllowCascadingDelete)

	r.syncLabel(log, nsInst, ns)

	// Sync all conditions. This should be placed at the end after all conditions are updated.
	r.syncConditions(log, inst, ns, hadCrit)
}

// syncOwner sets the parent to the owner and updates the HNS_MISSING condition
// if the HNS instance is missing in the owner namespace according to the forest.
// The namespace owner annotation is the source of truth of the ownership, since
// modifying a namespace has higher privilege than what HNC users can do.
func (r *HierarchyConfigReconciler) syncOwner(log logr.Logger, inst *api.HierarchyConfiguration, nsInst *corev1.Namespace, ns *forest.Namespace) {
	// Clear the HNS_MISSING condition if this is not an owned namespace or to
	// reset it for the updated condition later.
	ns.ClearConditionsByCode(log, api.HNSMissing)
	nm := ns.Name()
	onm := nsInst.Annotations[api.AnnotationOwner]
	ons := r.Forest.Get(onm)

	if onm == "" {
		ns.IsOwned = false
		return
	}

	ns.IsOwned = true

	if inst.Spec.Parent != onm {
		log.Info("The parent doesn't match the owner. Setting the owner as the parent.", "parent", inst.Spec.Parent, "owner", onm)
		inst.Spec.Parent = onm
	}

	// Look up the HNSes in the owner namespace. Set HNS_MISSING condition if it's
	// not there.
	found := false
	for _, hnsnm := range ons.HNSes {
		if hnsnm == nm {
			found = true
			break
		}
	}
	if !found {
		ns.SetCondition(api.NewAffectedNamespace(onm), api.HNSMissing, "The HNS instance is missing in the owner namespace")
	}
}

// markExisting marks the namespace as existing. If this is the first time we're reconciling this namespace,
// mark all possible relatives as being affected since they may have been waiting for this namespace.
func (r *HierarchyConfigReconciler) markExisting(log logr.Logger, ns *forest.Namespace) {
	if ns.SetExists() {
		log.Info("Reconciling new namespace")
		r.enqueueAffected(log, "relative of newly synced/created namespace", ns.RelativesNames()...)
		if ns.IsOwned {
			r.enqueueAffected(log, "owner of the newly synced/created namespace", ns.Parent().Name())
			r.hnsr.enqueue(log, ns.Name(), ns.Parent().Name(), "the missing owned namespace is found")
		}
	}
}

func (r *HierarchyConfigReconciler) syncParent(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace) {
	// Sync this namespace with its current parent.
	var curParent *forest.Namespace
	if inst.Spec.Parent != "" {
		curParent = r.Forest.Get(inst.Spec.Parent)
		if !curParent.Exists() {
			log.Info("Missing parent", "parent", inst.Spec.Parent)
			ns.SetLocalCondition(api.CritParentMissing, "missing parent")
		}
	}

	// Update the in-memory hierarchy if it's changed
	oldParent := ns.Parent()
	if oldParent != curParent {
		log.Info("Updating parent", "old", oldParent.Name(), "new", curParent.Name())
		if err := ns.SetParent(curParent); err != nil {
			log.Info("Couldn't update parent", "reason", err, "parent", inst.Spec.Parent)
			ns.SetLocalCondition(api.CritParentInvalid, err.Error())
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

// syncHNSes updates the HNS list. If any HNS is created/deleted, it will enqueue
// the child to update its HNS_MISSING condition. A modified HNS will appear
// twice in the change list (one in deleted, one in created), both owned namespace
// needs to be enqueued in this case.
func (r *HierarchyConfigReconciler) syncHNSes(log logr.Logger, ns *forest.Namespace, hnsnms []string) {
	for _, changedHNS := range ns.SetHNSes(hnsnms) {
		r.enqueueAffected(log, "the HNS instance is created/deleted", changedHNS)
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

func (r *HierarchyConfigReconciler) syncConditions(log logr.Logger, inst *api.HierarchyConfiguration, ns *forest.Namespace, hadCrit bool) {
	// Sync critical conditions after all locally-set conditions are updated.
	r.syncCritConditions(log, ns, hadCrit)

	// Convert and pass in-memory conditions to HierarchyConfiguration object.
	inst.Status.Conditions = ns.Conditions()
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
	if inst.CreationTimestamp.IsZero() {
		log.Info("Creating singleton on apiserver", "conditions", len(inst.Status.Conditions))
		if err := r.Create(ctx, inst); err != nil {
			log.Error(err, "while creating on apiserver")
			return false, err
		}
	} else {
		log.Info("Updating singleton on apiserver", "conditions", len(inst.Status.Conditions))
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
		return nil, err
	}
	return ns, nil
}

// getHierarchicalNamespaceNames returns a list of HierarchicalNamespace
// instance names in the given namespace.
func (r *HierarchyConfigReconciler) getHierarchicalNamespaceNames(ctx context.Context, nm string) ([]string, error) {
	var hnsnms []string

	// List all the hns instance in the namespace.
	ul := &unstructured.UnstructuredList{}
	ul.SetKind(api.HierarchicalNamespacesKind)
	ul.SetAPIVersion(api.HierarchicalNamespacesAPIVersion)
	if err := r.List(ctx, ul, client.InNamespace(nm)); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
		return hnsnms, nil
	}

	// Create a list of strings of the hns names.
	for _, inst := range ul.Items {
		hnsnms = append(hnsnms, inst.GetName())
	}

	return hnsnms, nil
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
	// Maps a HierarchicalNamespace (HNS) instance to the owner singleton.
	hnsMapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      api.Singleton,
					Namespace: a.Meta.GetNamespace(),
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
		Watches(&source.Kind{Type: &api.HierarchicalNamespace{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: hnsMapFn}).
		WithOptions(opts).
		Complete(r)
}
