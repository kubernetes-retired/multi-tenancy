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

	"github.com/go-logr/logr"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/metadata"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/object"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/stats"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// action is the action to take after Reconcile syncs with the in-memory forest.
// This is introduced to consolidate calls with forest lock.
type action int

const (
	// Start with an “unknown” to be sure of enum’s initialization.
	unknown action = iota
	remove
	write
	ignore
)

// ObjectReconciler reconciles generic propagated objects. You must create one for each
// group/version/kind that needs to be propagated and set its `GVK` field appropriately.
type ObjectReconciler struct {
	client.Client
	Log logr.Logger

	// Forest is the in-memory forest managed by the HierarchyReconciler.
	Forest *forest.Forest

	// GVK is the group/version/kind handled by this reconciler.
	GVK schema.GroupVersionKind

	// Affected is a channel of event.GenericEvent (see "Watching Channels" in
	// https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html) that is used to
	// enqueue additional objects that need updating.
	Affected chan event.GenericEvent

	// AffectedNamespace is a channel of events used to update namespaces.
	AffectedNamespace chan event.GenericEvent
}

// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete

// SyncNamespace can be called manually by the HierarchyReconciler when the hierarchy changes.
// It enqueues all the current objects in the namespace and local copies of the original objects
// in the ancestors.
func (r *ObjectReconciler) SyncNamespace(ctx context.Context, log logr.Logger, ns string) error {
	log = log.WithValues("gvk", r.GVK)

	// Enqueue all the current objects in the namespace because some of them may have been deleted.
	if err := r.enqueueLocalObjects(ctx, log, ns); err != nil {
		return err
	}

	// Enqueue local copies of the originals in the ancestors to catch any new or changed objects.
	r.enqueuePropagatedObjects(ctx, log, ns)

	return nil
}

func (r *ObjectReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	if ex[req.Namespace] {
		return ctrl.Result{}, nil
	}

	stats.StartObjReconcile(r.GVK)
	defer stats.StopObjReconcile(r.GVK)

	resp := ctrl.Result{}
	ctx := context.Background()
	log := r.Log.WithValues("trigger", req.NamespacedName)

	// Read the object. Sync it with the forest whether it's found/missing.
	inst := &unstructured.Unstructured{}
	inst.SetGroupVersionKind(r.GVK)
	inst.SetNamespace(req.Namespace)
	inst.SetName(req.Name)
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Couldn't read")
			return resp, err
		}
	}
	act, srcInst := r.syncWithForest(ctx, log, inst)

	return resp, r.operate(ctx, log, act, inst, srcInst)
}

// syncWithForest syncs the object instance with the in-memory forest. It returns the action to take on
// the object (delete, write or do nothing) and a source object if the action is to write it. It can
// also update the forest if a source object is added or removed.
func (r *ObjectReconciler) syncWithForest(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) (action, *unstructured.Unstructured) {
	// This is the only place we should lock the forest in each Reconcile, so this fn needs to return
	// everything relevant for the rest of the Reconcile. This fn shouldn't contact the apiserver since
	// that's a slow operation and everything will block on the lock being held.
	r.Forest.Lock()
	defer r.Forest.Unlock()

	// Early exit if no action is required.
	if r.ignore(ctx, log, inst) {
		return ignore, nil
	}

	// Clear any existing conditions associated with this object. TODO: this will never be called if
	// the source namespace holding this object goes away (i.e., if this namespace gets a new parent).
	// See https://github.com/kubernetes-sigs/multi-tenancy/issues/328.
	r.clearConditions(log, inst)

	// If an object doesn't exist, assume it's been deleted or not yet created.
	// inst.GetCreationTimestamp().IsZero() has compile time errors, so we manually check
	// if the CreationTimestamp is set. If yes, the object exists.
	exist := inst.GetCreationTimestamp() != v1.Time{}
	if !exist {
		// If it's a source, it must have been deleted. Update the forest and enqueue all its descendants.
		if r.isInForest(inst) {
			r.syncDeletedSource(ctx, log, inst)
			return ignore, nil
		}

		// This is a non-existent yet propagated object. Set "api.LabelInheritedFrom" label.
		// The correct value will be set in the "write" function.
		metadata.SetLabel(inst, api.LabelInheritedFrom, "sns placeholder")
	}

	return r.syncObject(ctx, log, inst)
}

func (r *ObjectReconciler) ignore(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) bool {
	// If it's about to be deleted, do nothing, just wait for it to be actually deleted.
	if !inst.GetDeletionTimestamp().IsZero() {
		return true
	}

	ns := r.Forest.Get(inst.GetNamespace())
	// If the object is reconciled before the namespace is synced (when start-up), do nothing.
	if !ns.Exists() {
		log.V(1).Info("Containing namespace hasn't been synced yet")
		return true
	}
	// If the namespace has critical condition, do nothing.
	if ns.HasCritCondition() {
		log.V(1).Info("Containing namespace has critical condition(s)")
		return true
	}

	return false
}

// syncObject handles a source object and a propagated object differently. If a source object changes,
// all descendant copies will be enqueued. If a propagated object is obsolete, it will be deleted.
// Otherwise, it will be overwritten by the source if they are different.
func (r *ObjectReconciler) syncObject(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) (action, *unstructured.Unstructured) {
	// If for some reason this has been called on an object that isn't namespaced, let's generate some
	// logspam!
	if inst.GetNamespace() == "" {
		for i := 0; i < 100; i++ {
			log.Info("Non-namespaced object!!!")
		}
		return ignore, nil
	}

	// This object is the source if it doesn't have the "api.LabelInheritedFrom" label.
	if !hasPropagatedLabel(inst) {
		r.syncSource(ctx, log, inst)
		// No action needs to take on source object, so early exit.
		return ignore, nil
	}

	// This object is a propagated copy.
	return r.syncPropagated(ctx, log, inst)
}

// syncPropagated will determine whether to delete the obsolete copy or overwrite it with the source.
// Or do nothing if it remains the same as the source object.
func (r *ObjectReconciler) syncPropagated(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) (action, *unstructured.Unstructured) {
	srcInst := r.Forest.Get(inst.GetNamespace()).GetSource(r.GVK, inst.GetName())

	// Return the action to delete the obsolete copy if there's no source in the ancestors.
	if srcInst == nil {
		return remove, nil
	}

	// If the copy is different from the source, return the write action and the source instance.
	if !reflect.DeepEqual(object.Canonical(inst), object.Canonical(srcInst)) {
		metadata.SetLabel(inst, api.LabelInheritedFrom, srcInst.GetNamespace())
		return write, srcInst
	}

	return ignore, nil
}

// syncSource syncs the copy in the forest with the current source object. If there's a change,
// enqueue all the descendants to propagate the new source.
func (r *ObjectReconciler) syncSource(ctx context.Context, log logr.Logger, src *unstructured.Unstructured) {
	// Note that we only call exclude() here, not in syncPropagated, because we'll never propagate an
	// *uncreated* excluded object, and if an excluded object somehow got propagated, we do want to
	// delete it.
	if r.exclude(log, src) {
		// In case this was previously propagated, we should delete any propagated copies
		r.syncDeletedSource(ctx, log, src)
		return
	}
	sns := src.GetNamespace()
	n := src.GetName()
	origCopy := r.Forest.Get(sns).GetOriginalObject(r.GVK, n)

	// Early exit if the source object exists and remains unchanged.
	if origCopy != nil && reflect.DeepEqual(object.Canonical(src), object.Canonical(origCopy)) {
		log.V(1).Info("Unchanged Source")
		return
	}

	// Update or create a copy of the source object in the forest
	r.Forest.Get(sns).SetOriginalObject(src.DeepCopy())

	// Enqueue all the descendant copies
	r.enqueueDescendants(ctx, log, src)
}

func (r *ObjectReconciler) enqueueDescendants(ctx context.Context, log logr.Logger, src *unstructured.Unstructured) {
	sns := src.GetNamespace()
	dns := r.Forest.Get(sns).DescendantNames()
	for _, ns := range dns {
		dc := object.Canonical(src)
		dc.SetNamespace(ns)
		log.V(1).Info("Enqueuing descendant copy", "affected", ns+"/"+src.GetName(), "reason", "The source changed")
		r.Affected <- event.GenericEvent{Meta: dc}
	}
}

func (r *ObjectReconciler) enqueueNamespace(log logr.Logger, nnm, reason string) {
	go func() {
		log.Info("Enqueuing for reconciliation", "affected", nnm, "reason", reason)
		// The handler only cares about the metadata
		inst := &api.HierarchyConfiguration{}
		inst.ObjectMeta.Name = api.Singleton
		inst.ObjectMeta.Namespace = nnm
		r.AffectedNamespace <- event.GenericEvent{Meta: inst}
	}()
}

// enqueueLocalObjects enqueues all the objects (with the same GVK) in the namespace.
func (r *ObjectReconciler) enqueueLocalObjects(ctx context.Context, log logr.Logger, ns string) error {
	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(r.GVK)
	if err := r.List(ctx, ul, client.InNamespace(ns)); err != nil {
		log.Error(err, "Couldn't list objects")
		return err
	}
	for _, inst := range ul.Items {
		// We don't need the entire canonical object here but only its metadata.
		// Using canonical copy is the easiest way to get an object with its metadata set.
		co := object.Canonical(&inst)
		co.SetNamespace(inst.GetNamespace())
		log.V(1).Info("Enqueuing existing object for reconciliation", "affected", co.GetName())
		r.Affected <- event.GenericEvent{Meta: co}
	}

	return nil
}

// enqueuePropagatedObjects is only called from SyncNamespace. It's the only place a forest lock is
// needed in SyncNamespace, so we made it into a function with forest lock instead of holding the
// lock for the entire SyncNamespace.
func (r *ObjectReconciler) enqueuePropagatedObjects(ctx context.Context, log logr.Logger, ns string) {
	r.Forest.Lock()
	defer r.Forest.Unlock()

	// Enqueue local copies of the original objects in the ancestors from forest.
	o := r.Forest.Get(ns).GetPropagatedObjects(r.GVK)
	for _, obj := range o {
		lc := object.Canonical(obj)
		lc.SetNamespace(ns)
		log.V(1).Info("Enqueuing local copy of the ancestor original for reconciliation", "affected", lc.GetName())
		r.Affected <- event.GenericEvent{Meta: lc}
	}
}

// operate operates the action generated from syncing the object with the forest.
func (r *ObjectReconciler) operate(ctx context.Context, log logr.Logger, act action, inst, srcInst *unstructured.Unstructured) error {
	switch act {
	case ignore:
		return nil
	case remove:
		return r.delete(ctx, log, inst)
	case write:
		return r.write(ctx, log, inst, srcInst)
	default:
		// Generate log for any unset action.
		log.Error(nil, "ACTION UNSET!!")
		return nil
	}
}

func (r *ObjectReconciler) delete(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) error {
	log.V(1).Info("Deleting obsolete copy")
	stats.WriteObject(inst.GroupVersionKind())
	err := r.Delete(ctx, inst)
	if errors.IsNotFound(err) {
		log.V(1).Info("The obsolete copy doesn't exist, no more action needed")
		return nil
	} else if err != nil {
		r.setErrorConditions(log, nil, inst, "delete", err)
		return err
	}

	return nil
}

func (r *ObjectReconciler) write(ctx context.Context, log logr.Logger, inst, srcInst *unstructured.Unstructured) error {
	// The object exists if CreationTimestamp is set. This flag enables us to have only 1 API call.
	exist := inst.GetCreationTimestamp() != v1.Time{}
	ns := inst.GetNamespace()
	inst = object.Canonical(srcInst)
	inst.SetNamespace(ns)
	metadata.SetLabel(inst, api.LabelInheritedFrom, srcInst.GetNamespace())
	log.V(1).Info("Writing", "dst", inst.GetNamespace(), "origin", srcInst.GetNamespace())

	var err error = nil
	var op string
	stats.WriteObject(inst.GroupVersionKind())
	if exist {
		err = r.Update(ctx, inst)
		op = "update"
	} else {
		err = r.Create(ctx, inst)
		op = "create"
	}
	if err != nil {
		r.setErrorConditions(log, srcInst, inst, op, err)
		log.Error(err, "Couldn't write", "object", inst)
	}
	return err
}

func (r *ObjectReconciler) setErrorConditions(log logr.Logger, srcInst, inst *unstructured.Unstructured, op string, err error) {
	r.Forest.Lock()
	defer r.Forest.Unlock()

	key := getObjectKey(inst)
	msg := fmt.Sprintf("Could not %s: %s", op, err.Error())
	r.setCondition(log, api.CannotUpdate, inst.GetNamespace(), key, msg)
	if srcInst != nil {
		r.setCondition(log, api.CannotPropagate, srcInst.GetNamespace(), key, msg)
	}
}

func (r *ObjectReconciler) setCondition(log logr.Logger, code api.Code, nnm, key, msg string) {
	r.Forest.Get(nnm).SetCondition(key, code, msg)
	r.enqueueNamespace(log, nnm, "Set condition for "+key+": "+msg)
}

func getObjectKey(inst *unstructured.Unstructured) string {
	gvk := inst.GetObjectKind().GroupVersionKind()
	return fmt.Sprintf("%s/%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, inst.GetNamespace(), inst.GetName())
}

func (r *ObjectReconciler) clearConditions(log logr.Logger, inst *unstructured.Unstructured) {
	gvk := inst.GetObjectKind().GroupVersionKind()
	key := fmt.Sprintf("%s/%s/%s/%s/%s", gvk.Group, gvk.Version, gvk.Kind, inst.GetNamespace(), inst.GetName())
	ns := r.Forest.Get(inst.GetNamespace())
	for ns != nil {
		if ns.ClearConditions(key, "") {
			// TODO: https://github.com/kubernetes-sigs/multi-tenancy/issues/326
			// Don't enqueue if we're just going to put the same conditions back
			r.enqueueNamespace(log, ns.Name(), "Removed conditions for "+key)
		}
		ns = ns.Parent()
	}
}

// hasPropagatedLabel returns true if "api.LabelInheritedFrom" label is set.
func hasPropagatedLabel(inst *unstructured.Unstructured) bool {
	labels := inst.GetLabels()
	if labels == nil {
		// this cannot be a copy
		return false
	}
	_, po := labels[api.LabelInheritedFrom]
	return po
}

// isInForest returns true if the object is found in the forest.
func (r *ObjectReconciler) isInForest(inst *unstructured.Unstructured) bool {
	ns := inst.GetNamespace()
	n := inst.GetName()
	gvk := inst.GroupVersionKind()
	return r.Forest.Get(ns).HasOriginalObject(gvk, n)
}

// syncDeletedSource deletes the source copy in the forest and then enqueues all its descendants.
func (r *ObjectReconciler) syncDeletedSource(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) {
	ns := inst.GetNamespace()
	n := inst.GetName()
	gvk := inst.GroupVersionKind()
	r.Forest.Get(ns).DeleteOriginalObject(gvk, n)
	r.enqueueDescendants(ctx, log, inst)
}

// exclude returns true if the object shouldn't be handled by the HNC, and in some non-obvious cases
// sets a condition on the namespace. Eventually, this may be user-configurable, but right now it's
// used for Service Account token secrets and to decide object propagation based on finalizer field.
func (r *ObjectReconciler) exclude(log logr.Logger, inst *unstructured.Unstructured) bool {
	// Object with nonempty finalizer list is not propagated
	if len(inst.GetFinalizers()) != 0 {
		r.setCondition(log, api.CannotPropagate, inst.GetNamespace(), getObjectKey(inst), "Objects with finalizers cannot be propagated")
		return true
	}

	switch {
	case r.GVK.Group == "" && r.GVK.Kind == "Secret":
		// These are reaped by a builtin K8s controller so there's no point copying them.
		// More to the point, SA tokens really aren't supposed to be copied between
		// namespaces. You *could* make the argument that a parent namespace's SA should be
		// shared with all its descendants, but you could also make the case that while
		// administration should be inherited, identity should not. At any rate, it's moot
		// as long as K8s auto deletes these tokens, and we shouldn't fight K8s.
		if inst.UnstructuredContent()["type"] == "kubernetes.io/service-account-token" {
			log.V(1).Info("Excluding: service account token")
			return true
		}
		return false

	default:
		return false
	}
}

func (r *ObjectReconciler) SetupWithManager(mgr ctrl.Manager, maxReconciles int) error {
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(r.GVK)
	opts := controller.Options{
		MaxConcurrentReconciles: maxReconciles,
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(target).
		Watches(&source.Channel{Source: r.Affected}, &handler.EnqueueRequestForObject{}).
		WithOptions(opts).
		Complete(r)
}
