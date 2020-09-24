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
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/config"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/metadata"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/object"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/stats"
)

// syncAction is the action to take after Reconcile syncs with the in-memory forest.
// This is introduced to consolidate calls with forest lock.
type syncAction string

const (
	actionUnknown syncAction = "<hnc internal error - unknown action>"
	actionRemove  syncAction = "remove"
	actionWrite   syncAction = "write"
	actionNop     syncAction = "no-op"

	unknownSourceNamespace = "<unknown-source-namespace>"
)

// namespacedNameSet is used to keep track of existing propagated objects of
// a specific GVK in the cluster.
type namespacedNameSet map[types.NamespacedName]bool

// ObjectReconciler reconciles generic propagated objects. You must create one for each
// group/version/kind that needs to be propagated and set its `GVK` field appropriately.
type ObjectReconciler struct {
	client.Client
	Log logr.Logger

	// Forest is the in-memory forest managed by the HierarchyConfigReconciler.
	Forest *forest.Forest

	// GVK is the group/version/kind handled by this reconciler.
	GVK schema.GroupVersionKind

	// Mode describes propagation mode of objects that are handled by this reconciler.
	// See more details in the comments of api.SynchronizationMode.
	Mode api.SynchronizationMode

	// Affected is a channel of event.GenericEvent (see "Watching Channels" in
	// https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html) that is used to
	// enqueue additional objects that need updating.
	Affected chan event.GenericEvent

	// AffectedNamespace is a channel of events used to update namespaces.
	AffectedNamespace chan event.GenericEvent

	// propagatedObjectsLock is used to prevent the race condition between concurrent reconciliation threads
	// trying to update propagatedObjects at the same time.
	propagatedObjectsLock sync.Mutex

	// propagatedObjects contains all propagated objects of the GVK handled by this reconciler.
	propagatedObjects namespacedNameSet
}

// HNC doesn't actually need all these permissions, but we *do* need to have them to be able to
// propagate RoleBindings for them. These match the permissions required by the builtin `admin`
// role, as seen in hack/test-issue-772.sh.
//
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete;deletecollection;impersonate

// SyncNamespace can be called manually by the HierarchyConfigReconciler when the hierarchy changes.
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

// GetGVK provides GVK that is handled by this reconciler.
func (r *ObjectReconciler) GetGVK() schema.GroupVersionKind {
	return r.GVK
}

// GetMode provides the mode of objects that are handled by this reconciler.
func (r *ObjectReconciler) GetMode() api.SynchronizationMode {
	return r.Mode
}

// GetValidateMode returns a valid api.SynchronizationMode based on the given mode. Please
// see the comments of api.SynchronizationMode for currently supported modes.
// If mode is not set, it will be api.Propagate by default. Any unrecognized mode is
// treated as api.Ignore.
func GetValidateMode(mode api.SynchronizationMode, log logr.Logger) api.SynchronizationMode {
	switch mode {
	case api.Propagate, api.Ignore, api.Remove:
		return mode
	case "":
		log.Info("Unset mode; using 'propagate'")
		return api.Propagate
	default:
		log.Info("Unrecognized mode; using 'ignore'", "mode", mode)
		return api.Ignore
	}
}

// SetMode sets the Mode field of an object reconciler and syncs objects in the cluster if needed.
// The method will return an error if syncs fail.
func (r *ObjectReconciler) SetMode(ctx context.Context, mode api.SynchronizationMode, log logr.Logger) error {
	log = log.WithValues("gvk", r.GVK)
	newMode := GetValidateMode(mode, log)
	oldMode := r.Mode
	if newMode == oldMode {
		return nil
	}
	log.Info("Changing mode of the object reconciler", "old", oldMode, "new", newMode)
	r.Mode = newMode
	// If the new mode is not "ignore", we need to update objects in the cluster
	// (e.g., propagate or remove existing objects).
	if newMode != api.Ignore {
		err := r.enqueueAllObjects(ctx, r.Log)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetNumPropagatedObjects returns the number of propagated objects of the GVK handled by this object reconciler.
func (r *ObjectReconciler) GetNumPropagatedObjects() int {
	r.propagatedObjectsLock.Lock()
	defer r.propagatedObjectsLock.Unlock()

	return len(r.propagatedObjects)
}

// enqueueAllObjects enqueues all the current objects in all namespaces.
func (r *ObjectReconciler) enqueueAllObjects(ctx context.Context, log logr.Logger) error {
	keys := r.Forest.GetNamespaceNames()
	for _, ns := range keys {
		// Enqueue all the current objects in the namespace.
		if err := r.enqueueLocalObjects(ctx, log, ns); err != nil {
			log.Error(err, "Error while trying to enqueue local objects", "namespace", ns)
			return err
		}
	}
	return nil
}

func (r *ObjectReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	resp := ctrl.Result{}
	ctx := context.Background()
	log := r.Log.WithValues("trigger", req.NamespacedName)

	if config.EX[req.Namespace] {
		return resp, nil
	}

	if r.Mode == api.Ignore {
		return resp, nil
	}

	stats.StartObjReconcile(r.GVK)
	defer stats.StopObjReconcile(r.GVK)

	// Read the object.
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

	// Sync with the forest and perform any required actions.
	actions, srcInst := r.syncWithForest(ctx, log, inst)
	return resp, r.operate(ctx, log, actions, inst, srcInst)
}

// syncWithForest syncs the object instance with the in-memory forest. It returns the action to take on
// the object (delete, write or do nothing) and a source object if the action is to write it. It can
// also update the forest if a source object is added or removed.
func (r *ObjectReconciler) syncWithForest(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) (syncAction, *unstructured.Unstructured) {
	// This is the only place we should lock the forest in each Reconcile, so this fn needs to return
	// everything relevant for the rest of the Reconcile. This fn shouldn't contact the apiserver since
	// that's a slow operation and everything will block on the lock being held.
	r.Forest.Lock()
	defer r.Forest.Unlock()

	// If this namespace isn't ready to be synced (or is never synced), early exit. We'll be called
	// again if this changes.
	if r.skipNamespace(ctx, log, inst) {
		return actionNop, nil
	}

	// If the object's missing and we know how to handle it, return early.
	if missingAction := r.syncMissingObject(ctx, log, inst); missingAction != actionUnknown {
		return missingAction, nil
	}

	// Update the forest and get the intended action.
	action, srcInst := r.syncObject(ctx, log, inst)

	// If the namespace has a critical condition, we shouldn't actually take any action, regardless of
	// what we'd _like_ to do. We still needed to sync the forest since we want to know when objects
	// are added and removed, so we can sync them properly if the critical condition is resolved, but
	// don't do anything else for now.
	if ca := r.Forest.Get(inst.GetNamespace()).GetCritAncestor(); ca != "" {
		log.Info("Suppressing action due to critical condition", "critAncestor", ca, "action", action)
		return actionNop, nil
	}

	return action, srcInst
}

func (r *ObjectReconciler) skipNamespace(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) bool {
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

	return false
}

// syncMissingObject handles the case where the object we're reconciling doesn't exist. If it can
// determine a final action to take, it returns that action, otherwise it returns actionUnknown
// which indicates that we need to call the regular syncObject method. Note that this method may
// modify `inst`.
func (r *ObjectReconciler) syncMissingObject(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) syncAction {
	// If the object exists, skip
	if inst.GetCreationTimestamp() != (v1.Time{}) {
		return actionUnknown
	}

	// If it's a source, it must have been deleted. Update the forest and enqueue all its
	// descendants, but there's nothing else to do.
	if r.forestHasSource(inst) {
		r.syncUnpropagatedSource(ctx, log, inst)
		return actionNop
	}

	// This object doesn't exist, and yet someone thinks we need to reconcile it. There are a few
	// reasons why this can happen:
	//
	// 1. This was a source object, and for some reason we got the notification that it's been
	// deleted twice.
	// 2. This is a propagated object. We haven't actually created it yet, but its source exists in
	// the forest so we need to make a copy.
	// 3a. This *was* a propagated object that we've deleted, and we're getting a notification about
	// it.
	// 3b. This *should have been* a propagated object, but due to some error we were never able to
	// create it.
	//
	// In all cases, we're going to give it the api.LabelInherited from label with a dummy value, so
	// that syncObject() treats it as though it's a propagated object. This works well in all three
	// cases because:
	//
	// 1. If this was a source object, but we treat it as a propagated object, we'll see that
	// there's no source in the tree and so there will be nothing to do (which is correct).
	// 2. If this really is a propagated object that needs to be created, we'll find the source in
	// the tree and call write(), which will set the correct value for LabelInheritedFrom.
	// 3. If this *was* a propagated object that's been deleted, then we'll see there's no source
	// (like in case #1) and ignore it.
	metadata.SetLabel(inst, api.LabelInheritedFrom, unknownSourceNamespace)

	// Continue the regular syncing process
	return actionUnknown
}

// syncObject determines if this object is a source or propagated copy and handles it accordingly.
func (r *ObjectReconciler) syncObject(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) (syncAction, *unstructured.Unstructured) {
	// If for some reason this has been called on an object that isn't namespaced, let's generate some
	// logspam!
	if inst.GetNamespace() == "" {
		for i := 0; i < 100; i++ {
			log.Info("Non-namespaced object!!!")
		}
		return actionNop, nil
	}

	// This object is a propagated copy if it has "api.LabelInheritedFrom" label.
	if hasPropagatedLabel(inst) {
		return r.syncPropagated(ctx, log, inst)
	}

	// Find the source object of the same name in the ancestors from top down to
	// see if there's a conflicting source.
	srcInst := r.Forest.Get(inst.GetNamespace()).GetSource(r.GVK, inst.GetName())

	// The object is a source without conflict if a copy of the source is not
	// found in the forest or itself is found.
	if srcInst == nil || srcInst.GetNamespace() == inst.GetNamespace() {
		r.syncSource(ctx, log, inst)
		// No action needs to take on source objects.
		return actionNop, nil
	}

	// Since there's a conflict that another source with the same name is found in
	// the ancestors, this instance will be treated as propagated objects and will
	// be overwritten by the source in the ancestor.
	return r.syncPropagated(ctx, log, inst)
}

// syncPropagated will determine whether to delete the obsolete copy or overwrite it with the source.
// Or do nothing if it remains the same as the source object.
func (r *ObjectReconciler) syncPropagated(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) (syncAction, *unstructured.Unstructured) {
	// Find the source object of the same name in the ancestors from top down.
	srcInst := r.Forest.Get(inst.GetNamespace()).GetSource(r.GVK, inst.GetName())

	// If no source object exists, delete this object. This can happen when the source was deleted by
	// users or the admin decided this type should no longer be propagated.
	if srcInst == nil {
		return actionRemove, nil
	}
	// If an object doesn't exist, assume it's been deleted or not yet created.
	exists := inst.GetCreationTimestamp() != v1.Time{}
	selector := r.getSelector(ctx, log, srcInst)
	log.Info("JI:", "Selector", selector)

	nsLabels := labels.Set(r.Forest.Get(inst.GetNamespace()).GetLabels())
	if selector != nil && !selector.Matches(nsLabels) {
		log.Info("JI: Selector label does not match")
		if exists {
			log.Info("JI: Removing object")
			// The object already exists but it shouldn't be here according to the updated selector, so we remove it
			r.deleteObject(ctx, log, inst)
			return actionRemove, nil
		} else {
			// The object already exists and doesn't need to be updated. This will typically happen when HNC
			// is restarted - all the propagated objects already exist on the apiserver. Record that it exists
			// for our statistics.
			r.recordPropagatedObject(log, inst.GetNamespace(), inst.GetName())

			// Nothing more needs to be done.
			return actionNop, nil
		}
	}

	// If the copy does not exist, or is different from the source, return the write action and the
	// source instance. Note that DeepEqual could return `true` even if the object doesn't exist if
	// the source object is trivial (e.g. a completely empty ConfigMap).
	if !exists ||
		!reflect.DeepEqual(object.Canonical(inst), object.Canonical(srcInst)) ||
		inst.GetLabels()[api.LabelInheritedFrom] != srcInst.GetNamespace() {
		metadata.SetLabel(inst, api.LabelInheritedFrom, srcInst.GetNamespace())
		return actionWrite, srcInst
	}

	// Nothing more needs to be done.
	return actionNop, nil
}

func (r *ObjectReconciler) getSelector(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) labels.Selector {
	annot := inst.GetAnnotations()
	selectorStr, ok := annot[api.AnnotationSelector]
	if !ok {
		return nil
	}
	labelSelector, err := v1.ParseToLabelSelector(selectorStr)
	// TODO: surface the error messages here
	if err != nil {
		log.Error(err, "Could not parse selector annotation to labelSelector")
		return nil
	}
	selector, err := v1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		log.Error(err, "Could not convert labelSelector to selector")
	}
	return selector
}

// syncSource syncs the copy in the forest with the current source object. If there's a change,
// enqueue all the descendants to propagate the new source.
func (r *ObjectReconciler) syncSource(ctx context.Context, log logr.Logger, src *unstructured.Unstructured) {
	if !r.shouldPropagateSource(log, src) {
		// If an object was *previously* propagated by HNC, it will already be in the forest, and all
		// the propagated copies will think they can stick around too. If, for some reason, we _now_
		// want to stop propagating it (e.g. because we've changed the sync mode in the HNCConfig
		// object), we need to delete it from the forest and enqueue all its propagated copies. The
		// propagated copies will then see that that the source is missing from the forest, and delete
		// themselves.
		r.syncUnpropagatedSource(ctx, log, src)
		return
	}

	nnm := src.GetNamespace()
	nm := src.GetName()
	ns := r.Forest.Get(nnm)
	origCopy := ns.GetOriginalObject(r.GVK, nm)

	// Early exit if the source object exists and remains unchanged.
	if origCopy != nil && reflect.DeepEqual(object.Canonical(src), object.Canonical(origCopy)) {
		log.V(1).Info("Unchanged Source")
		return
	}

	// Update or create a copy of the source object in the forest
	ns.SetOriginalObject(src.DeepCopy())

	// Signal the config reconciler for reconciliation because it is possible that a source object is
	// added to the apiserver.
	hnccrSingleton.requestReconcile("possible new source object")

	// Enqueue all the descendant copies
	r.enqueueDescendants(ctx, log, src, "new or modified source object")
}

func (r *ObjectReconciler) enqueueDescendants(ctx context.Context, log logr.Logger, src *unstructured.Unstructured, reason string) {
	sns := r.Forest.Get(src.GetNamespace())
	if ca := sns.GetCritAncestor(); ca != "" {
		// There's no point enqueuing anything if the source namespace has a crit condition since we'll
		// just skip any action anyway.
		log.Info("Will not enqueue descendants due to crit ancestor", "critAncestor", ca, "oldReason", reason)
		return
	}
	log.Info("Enqueuing descendant objects", "reason", reason)
	for _, ns := range sns.DescendantNames() {
		dc := object.Canonical(src)
		dc.SetNamespace(ns)
		log.V(1).Info("Enqueuing descendant copy", "affected", ns+"/"+src.GetName(), "reason", reason)
		r.Affected <- event.GenericEvent{Meta: dc}
	}
}

func (r *ObjectReconciler) enqueueHC(log logr.Logger, nnm, reason string) {
	go func() {
		log.Info("Enqueuing HierarchyConfiguration for reconciliation", "ns", nnm, "reason", reason)
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
func (r *ObjectReconciler) operate(ctx context.Context, log logr.Logger, act syncAction, inst, srcInst *unstructured.Unstructured) error {
	var err error

	switch act {
	case actionNop:
		// nop
	case actionRemove:
		err = r.deleteObject(ctx, log, inst)
	case actionWrite:
		err = r.writeObject(ctx, log, inst, srcInst)
	default: // this should never, ever happen. But if it does, try to make a very obvious error message.
		if act == "" {
			act = actionUnknown
		}
		err = fmt.Errorf("HNC couldn't determine how to update this object (desired action: %s)", act)
	}

	r.setConditions(log, srcInst, inst, act, err)
	return err
}

func (r *ObjectReconciler) deleteObject(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) error {
	log.V(1).Info("Deleting obsolete copy")
	stats.WriteObject(inst.GroupVersionKind())
	err := r.Delete(ctx, inst)
	if errors.IsNotFound(err) {
		log.V(1).Info("The obsolete copy doesn't exist, no more action needed")
		return nil
	}

	if err != nil {
		// Don't log the error since controller-runtime will do it for us
		return err
	}

	// Remove the propagated object from the map because we are confident that the object was successfully deleted
	// on the apiserver.
	log.Info("Deleted")
	r.recordRemovedObject(log, inst.GetNamespace(), inst.GetName())
	return nil
}

func (r *ObjectReconciler) writeObject(ctx context.Context, log logr.Logger, inst, srcInst *unstructured.Unstructured) error {
	// The object exists if CreationTimestamp is set. This flag enables us to have only 1 API call.
	exist := inst.GetCreationTimestamp() != v1.Time{}
	ns := inst.GetNamespace()
	inst = object.Canonical(srcInst)
	inst.SetNamespace(ns)
	metadata.SetLabel(inst, api.LabelInheritedFrom, srcInst.GetNamespace())
	log.V(1).Info("Writing", "dst", inst.GetNamespace(), "origin", srcInst.GetNamespace())

	var err error = nil
	stats.WriteObject(inst.GroupVersionKind())
	if exist {
		log.Info("Updating object")
		err = r.Update(ctx, inst)
		// RoleBindings can't have their Roles changed after they're created
		// (see  https://github.com/kubernetes-sigs/multi-tenancy/issues/798).
		// If an RB was quickly delete and re-created in an ancestor namespace
		// - fast enough that by the time that HNC notices, the new RB exists; or
		// if there's a change to the RBs when HNC isn't running - HNC could see
		// it as an update (not a delete + create) and attempt to update the RBs in
		// all descendant namespaces, and this will fail. In order to handle this
		// case, we try to delete and re-create the rolebinding here

		// We only found this issue with the RoleBinding object, but we *think* this
		// will also be helpful for other similar objects that end up with the same error
		// type. If we find out later that this assumption is not true, we can update the
		// logic here to only deal with RoleBinding.

		// The error type is 'Invalid' after I tested it out with different error types
		// from https://godoc.org/k8s.io/apimachinery/pkg/api/errors
		if err != nil && errors.IsInvalid(err) {
			if err = r.Delete(ctx, inst); err == nil {
				err = r.Create(ctx, inst)
				if err != nil {
					log.Info("Unable to create new object.") // error is handles below
				} else {
					log.Info("Couldn't update object but delete and re-create it.")
				}
			} else {
				log.Info("Unable to delete the existing object.") // error is handles below
			}
		}
	} else {
		log.Info("Creating object")
		err = r.Create(ctx, inst)
	}
	if err != nil {
		// Don't log the error since controller-runtime will do it for us
		return err
	}

	// Add the object to the map if it does not exist because we are confident that the object was updated/created
	// successfully on the apiserver.
	r.recordPropagatedObject(log, inst.GetNamespace(), inst.GetName())
	return nil
}

// setConditions is called when the reconciler has performed all necessary actions and knows if
// they've succeeded or failed. It re-locks the forest just long enough to set or clear all
// conditions. Since we didn't hold the lock while we were contacting the apiserver, we need to be
// aware that the hierarchy may have changed in arbitrary ways; see below for details.
//
// This function also enqueues any modified namespaces for reconciliation.
func (r *ObjectReconciler) setConditions(log logr.Logger, srcInst, inst *unstructured.Unstructured, act syncAction, err error) {
	r.Forest.Lock()
	defer r.Forest.Unlock()
	ao := api.NewAffectedObject(inst.GetObjectKind().GroupVersionKind(), inst.GetNamespace(), inst.GetName())
	// This affected object is initialized with the source object if it exits.
	sao := api.NewAffectedObject(inst.GetObjectKind().GroupVersionKind(), inst.GetLabels()[api.LabelInheritedFrom], inst.GetName())
	ns := r.Forest.Get(inst.GetNamespace())

	switch {
	case hasFinalizers(inst):
		// Propagated objects can never have finalizers
		if ns.SetCondition(ao, api.CannotPropagate, "Objects with finalizers cannot be propagated") {
			r.enqueueHC(log, ns.Name(), "Set CannotPropagate since it has finalizers")
		}

	case err != nil:
		// There was an error updating this object; set a condition pointing to the source object. Note we
		// never take actions on a source object, so only propagated objects could possibly get an error.
		msg := fmt.Sprintf("Could not %s: %s", act, err.Error())
		if ns.SetCondition(sao, api.CannotUpdate, msg) {
			r.enqueueHC(log, ns.Name(), "Set CannotUpdate due to error")
		}

		// Also set a condition on the source if one exists.
		if srcInst != nil {
			srcNS := r.Forest.Get(srcInst.GetNamespace())
			if ns.IsAncestor(srcNS) {
				if srcNS.SetCondition(ao, api.CannotPropagate, msg) {
					r.enqueueHC(log, srcNS.Name(), "Set CannotPropagate on source namespace due to error updating")
				}
			} else {
				// Because we released the lock for a bit, there's a chance that srcInst is no longer a parent
				// of inst. If that happened, any conditions on srcInst related to this object should already
				// have been cleared by the HCR.
				log.Info("Not setting conditions on source namespace since it's no longer an ancestor", "srcNS", srcNS.Name())
			}
		}

	default:
		// No error conditions exist for this object; clear all conditions in the namespace and all its
		// ancestors (technically, srcNS is the only feasible ancestor, but because we don't hold the
		// lock, it's safer to just do everything).
		if hasPropagatedLabel(inst) {
			cleared := clearConditionsForUnknownSources(log, ns, sao)
			if !cleared {
				// If there were no unknown sources, try the known one
				cleared = ns.ClearCondition(sao, "")
			}
			if cleared {
				r.enqueueHC(log, ns.Name(), "Removed condition on destination namespace")
			}
		}
		for ns != nil {
			if ns.ClearCondition(ao, "") {
				r.enqueueHC(log, ns.Name(), "Removed condition on source namespace")
			}
			ns = ns.Parent()
		}
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

// clearConditionsForUnknownSources cleans up conditions that indicated that we couldn't propagate an
// object, after the source object is deleted.
//
// This function is used when we're reconciling an object that doesn't actually exist, and that we
// think wasn't a source. In such cases, the earlier parts of this reconciler sets the inheritedFrom
// label to `unknownSourceNamespace`; if we had found a source, this label would have been updated,
// but it wasn't. So if this function doesn't early-exit, we know this either was a source namespace
// that got deleted (and then reconciled multiple times), or else it was supposed to be a propagated
// object that never actually got created, and whose source has now been deleted (if the source was
// present, the label would have been set).
//
// In this first case, this function will do nothing - any conditions associated with a source
// object will be cleared when the source was first deleted. But in the second case, the _reason_
// that this propagated object was never actually created is very likely because some error
// prevented it from being created, which means there's very likely a condition on the namespace.
//
// Unfortunately, namespaces use the source object as the key to handle conditions, and we don't
// know the source! So we can't just say ns.ClearCondition(sourceObject, "") as we usually would.
// Instead, we need to look through all relevant conditions on the namespace and clear anything that
// *might* have been caused by this object failing to be propagated.
//
// Is this really safe? Yes: HNC enforces that all objects with a given name and GVK are the same
// across namespaces, so any condition on this namespace with the same GVK and name *must* be
// talking about this copy of the object in this namespace. Since this object doesn't *and
// shouldn't* exist, any condition on its namespace are now invalid, and can be safely removed.
//
// Is this method fast enough? In theory. ns.GetConditions() looks slow, but in a typical case, most
// namespaces will so few conditions that looking through them in memory will take negligible time.
// Also, it's very rare that we'll reconcile a completely missing object in the first place, so this
// method should be caleld very rarely.
func clearConditionsForUnknownSources(log logr.Logger, ns *forest.Namespace, sao api.AffectedObject) bool {
	// Early-exit if the source is known
	if sao.Namespace != unknownSourceNamespace {
		return false
	}

	cleared := false
	for _, cond := range ns.Conditions() {
		for _, ao := range cond.Affects {
			aoCmp := ao
			aoCmp.Namespace = unknownSourceNamespace // to make comparison easier
			if aoCmp != sao {
				continue
			}

			log.Info("Object didn't exist but found possibly matching condition", "condition", cond)
			ns.ClearCondition(ao, "")
			cleared = true
		}
	}
	return cleared
}

// forestHasSource returns true if the original object is found in the forest.
func (r *ObjectReconciler) forestHasSource(inst *unstructured.Unstructured) bool {
	ns := inst.GetNamespace()
	n := inst.GetName()
	gvk := inst.GroupVersionKind()
	return r.Forest.Get(ns).HasOriginalObject(gvk, n)
}

// syncUnpropagatedSource deletes the source copy in the forest (if it exists) and then enqueues any
// propagated copies of the source.
//
// The method can be called when the source was deleted by users, or if it will no longer be
// propagated, e.g. because the user's changed the sync mode in the HNC Config.
func (r *ObjectReconciler) syncUnpropagatedSource(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) {
	nnm := inst.GetNamespace()
	nm := inst.GetName()
	gvk := inst.GroupVersionKind()
	r.Forest.Get(nnm).DeleteOriginalObject(gvk, nm)

	// Signal the config reconciler for reconciliation because it is possible that the source object is
	// deleted on the apiserver.
	hnccrSingleton.requestReconcile("possible deleted source object")

	r.enqueueDescendants(ctx, log, inst, "possibly deleted source object")
}

// shouldPropagateSource returns true if the object should be propagated by the HNC. The following
// objects are not propagated:
// - Objects of a type whose mode is set to "remove" in the HNCConfiguration singleton
// - Objects with nonempty finalizer list
// - Service Account token secrets
func (r *ObjectReconciler) shouldPropagateSource(log logr.Logger, inst *unstructured.Unstructured) bool {
	switch {
	// Users can set the mode of a type to "remove" to exclude objects of the type
	// from being handled by HNC.
	case r.Mode == api.Remove:
		return false

	// Object with nonempty finalizer list is not propagated
	case hasFinalizers(inst):
		return false

	case r.GVK.Group == "" && r.GVK.Kind == "Secret":
		// These are reaped by a builtin K8s controller so there's no point copying them.
		// More to the point, SA tokens really aren't supposed to be copied between
		// namespaces. You *could* make the argument that a parent namespace's SA should be
		// shared with all its descendants, but you could also make the case that while
		// administration should be inherited, identity should not. At any rate, it's moot
		// as long as K8s auto deletes these tokens, and we shouldn't fight K8s.
		if inst.UnstructuredContent()["type"] == "kubernetes.io/service-account-token" {
			log.V(1).Info("Excluding: service account token")
			return false
		}
		return true

	default:
		// Everything else is propagated
		return true
	}
}

// recordPropagatedObject records the fact that this object has been propagated, so we can report
// statistics in the HNC Config.
func (r *ObjectReconciler) recordPropagatedObject(log logr.Logger, namespace, name string) {
	r.propagatedObjectsLock.Lock()
	defer r.propagatedObjectsLock.Unlock()

	nnm := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if !r.propagatedObjects[nnm] {
		r.propagatedObjects[nnm] = true
		hnccrSingleton.requestReconcile("newly propagated object")
	}
}

// recordRemovedObject records the fact that this (possibly) previously propagated object no longer
// exists.
func (r *ObjectReconciler) recordRemovedObject(log logr.Logger, namespace, name string) {
	r.propagatedObjectsLock.Lock()
	defer r.propagatedObjectsLock.Unlock()

	nnm := types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
	if r.propagatedObjects[nnm] {
		delete(r.propagatedObjects, nnm)
		hnccrSingleton.requestReconcile("newly unpropagated object")
	}
}

func hasFinalizers(inst *unstructured.Unstructured) bool {
	return len(inst.GetFinalizers()) != 0
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
