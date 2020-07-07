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

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/config"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/metadata"
)

// AnchorReconciler reconciles SubnamespaceAnchor CRs to make sure all the subnamespaces are
// properly maintained.
type AnchorReconciler struct {
	client.Client
	Log logr.Logger

	forest *forest.Forest

	// Affected is a channel of event.GenericEvent (see "Watching Channels" in
	// https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html) that is used to
	// enqueue additional objects that need updating.
	Affected chan event.GenericEvent
}

// Reconcile sets up some basic variables and then calls the business logic. It currently
// only handles the creation of the namespaces but no deletion or state reporting yet.
func (r *AnchorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("trigger", req.NamespacedName)
	log.Info("Reconciling anchor")

	// Get names of the hierarchical namespace and the current namespace.
	nm := req.Name
	pnm := req.Namespace

	// Get instance from apiserver. If the instance doesn't exist, do nothing and early exist because
	// HCR watches anchor and (if applicable) has already updated the parent HC when this anchor was
	// purged.
	inst, err := r.getInstance(ctx, pnm, nm)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Report "Forbidden" state and early exit if the namespace is not allowed to have subnamespaces
	// but has bypassed the webhook and successfully created the anchor. Forbidden anchors won't have
	// finalizers.
	// TODO refactor/split the EX map for 1) reconciler exclusion and 2) subnamespaces exclusion
	// purposes. See issue: https://github.com/kubernetes-sigs/multi-tenancy/issues/495
	if config.EX[pnm] {
		inst.Status.State = api.Forbidden
		return ctrl.Result{}, r.writeInstance(ctx, log, inst)
	}

	// Get the subnamespace. If it doesn't exist, initialize one.
	snsInst, err := r.getNamespace(ctx, nm)
	if err != nil {
		return ctrl.Result{}, err
	}

	if deleting, err := r.onDeleting(ctx, log, inst, snsInst); deleting {
		return ctrl.Result{}, err
	}

	// Update the state. If the subnamespace doesn't exist, create it.
	// (This is how a subnamespace is created through a CR)
	r.updateState(log, inst, snsInst)
	if inst.Status.State == api.Missing {
		if err := r.writeNamespace(ctx, log, nm, pnm); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Add finalizers on all non-forbidden anchors to ensure it's not deleted until
	// after the subnamespace is deleted.
	inst.ObjectMeta.Finalizers = []string{api.MetaGroup}
	return ctrl.Result{}, r.writeInstance(ctx, log, inst)
}

func (r *AnchorReconciler) onDeleting(ctx context.Context, log logr.Logger, inst *api.SubnamespaceAnchor, snsInst *corev1.Namespace) (deleting bool, err error) {
	// Early exit and continue reconciliation if the instance is not being deleted.
	if inst.DeletionTimestamp.IsZero() {
		return false, nil
	}

	deletingCRD, err := isDeletingCRD(ctx, r, api.Anchors)
	if err != nil {
		log.Info("Couldn't determine if CRD is being deleted")
		return false, err
	}

	log.Info("The anchor is being deleted", "deletingCRD", deletingCRD)
	switch {
	case len(inst.ObjectMeta.Finalizers) == 0:
		// We've finished processing this, nothing to do.
		log.Info("Do nothing since the finalizers are already gone.")
		return true, nil
	case r.shouldDeleteSubns(inst, snsInst, deletingCRD):
		// The subnamespace is not already being deleted but it allows cascadingDelete or it's a leaf.
		// Delete the subnamespace, unless the CRD is being deleted, in which case, we want to leave the
		// namespaces alone.
		log.Info("The subnamespace is not being deleted but it allows cascading deletion or it's a leaf.")
		return true, r.deleteNamespace(ctx, log, snsInst)
	case r.removeFinalizers(log, inst, snsInst):
		// We've determined that this anchor is ready to be deleted.
		return true, r.writeInstance(ctx, log, inst)
	default:
		// The subnamespace is already being deleted. Do nothing in this Reconcile().
		// Wait until it's purged to remove the finalizers in another Reconcile().
		log.Info("Do nothing since the subnamespace is still being deleted (not purged yet).")
		return true, nil
	}
}

// shouldDeleteSubns returns true if the namespace still exists and it is a leaf
// subnamespace or it allows cascading delete unless the CRD is being deleted.
func (r *AnchorReconciler) shouldDeleteSubns(inst *api.SubnamespaceAnchor, nsInst *corev1.Namespace, deletingCRD bool) bool {
	r.forest.Lock()
	defer r.forest.Unlock()

	// If the CRD is being deleted, we want to leave the subnamespace alone.
	if deletingCRD {
		return false
	}

	cnm := inst.Name
	pnm := inst.Namespace
	cns := r.forest.Get(cnm)

	// If the declared subnamespace is not created by this anchor, don't delete it.
	if cns.Parent().Name() != pnm {
		return false
	}

	// If the subnamespace is created by this anchor but is already being deleted,
	// or has already been deleted, then there's no need to delete it again.
	if !nsInst.DeletionTimestamp.IsZero() || !cns.Exists() {
		return false
	}

	// The subnamespace exists and isn't being deleted. We should delete it if it
	// doesn't have any children itself, or if cascading deletion is enabled.
	return cns.ChildNames() == nil || cns.AllowsCascadingDelete()
}

func (r *AnchorReconciler) removeFinalizers(log logr.Logger, inst *api.SubnamespaceAnchor, snsInst *corev1.Namespace) bool {
	pnm := inst.Namespace
	sOf := snsInst.GetAnnotations()[api.SubnamespaceOf]

	// This switch statement has an explicit case for all conditions when we *can* delete the
	// finalizers. The default behaviour is that we *cannot* delete the finalizers yet.
	switch {
	case snsInst.Name == "":
		log.Info("The subnamespace is already purged; allowing the anchor to be deleted")
	case sOf != pnm:
		log.Info("The subnamespace believes it is the subnamespace of another namespace; allowing the anchor to be deleted", "annotation", sOf)
	case snsInst.DeletionTimestamp.IsZero():
		// onDeleting has decided not to try to delete this namespace. Either cascading deletion isn't
		// allowed (and the user has bypassed the webhook), or the CRD has been deleted and we don't
		// want to invoke cascading deletion. Just allow the anchor to be deleted.
		log.Info("We've decided not to delete the subnamespace; allowing the anchor to be deleted")
	default:
		return false
	}

	inst.ObjectMeta.Finalizers = nil
	return true
}

func (r *AnchorReconciler) updateState(log logr.Logger, inst *api.SubnamespaceAnchor, snsInst *corev1.Namespace) {
	nm := inst.Name
	pnm := inst.Namespace
	sOf := snsInst.Annotations[api.SubnamespaceOf]
	switch {
	case snsInst.Name == "":
		log.Info("The subnamespace does not exist", "subnamespace", nm)
		inst.Status.State = api.Missing
	case sOf != pnm:
		log.Info("The subnamespaceOf annotation of this namespace doesn't match its parent", "annotation", sOf)
		inst.Status.State = api.Conflict
	default:
		log.Info("The subnamespace has the correct subnamespaceOf annotation", "annotation", sOf)
		inst.Status.State = api.Ok
	}
}

// It enqueues a subnamespace anchor for later reconciliation. This occurs in a goroutine
// so the caller doesn't block; since the reconciler is never garbage-collected, this is safe.
func (r *AnchorReconciler) enqueue(log logr.Logger, nm, pnm, reason string) {
	go func() {
		// The watch handler doesn't care about anything except the metadata.
		inst := &api.SubnamespaceAnchor{}
		inst.ObjectMeta.Name = nm
		inst.ObjectMeta.Namespace = pnm
		log.Info("Enqueuing for reconciliation", "affected", pnm+"/"+nm, "reason", reason)
		r.Affected <- event.GenericEvent{Meta: inst}
	}()
}

func (r *AnchorReconciler) getInstance(ctx context.Context, pnm, nm string) (*api.SubnamespaceAnchor, error) {
	nsn := types.NamespacedName{Namespace: pnm, Name: nm}
	inst := &api.SubnamespaceAnchor{}
	if err := r.Get(ctx, nsn, inst); err != nil {
		return nil, err
	}
	return inst, nil
}

func (r *AnchorReconciler) writeInstance(ctx context.Context, log logr.Logger, inst *api.SubnamespaceAnchor) error {
	if inst.CreationTimestamp.IsZero() {
		log.Info("Creating instance on apiserver")
		if err := r.Create(ctx, inst); err != nil {
			log.Error(err, "while creating on apiserver")
			return err
		}
	} else {
		log.Info("Updating instance on apiserver")
		if err := r.Update(ctx, inst); err != nil {
			log.Error(err, "while updating on apiserver")
			return err
		}
	}
	return nil
}

// getNamespace returns the namespace if it exists, or returns an invalid, blank, unnamed one if it
// doesn't. This allows it to be trivially identified as a namespace that doesn't exist, and also
// allows us to easily modify it if we want to create it.
func (r *AnchorReconciler) getNamespace(ctx context.Context, nm string) (*corev1.Namespace, error) {
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

func (r *AnchorReconciler) writeNamespace(ctx context.Context, log logr.Logger, nm, pnm string) error {
	inst := &corev1.Namespace{}
	inst.ObjectMeta.Name = nm
	metadata.SetAnnotation(inst, api.SubnamespaceOf, pnm)

	// It's safe to use create here since if the namespace is created by someone
	// else while this reconciler is running, returning an error will trigger a
	// retry. The reconciler will set the 'Conflict' state instead of recreating
	// this namespace. All other transient problems should trigger a retry too.
	log.Info("Creating namespace on apiserver")
	if err := r.Create(ctx, inst); err != nil {
		log.Error(err, "while creating on apiserver")
		return err
	}
	return nil
}

func (r *AnchorReconciler) deleteNamespace(ctx context.Context, log logr.Logger, inst *corev1.Namespace) error {
	log.Info("Deleting namespace on apiserver")
	if err := r.Delete(ctx, inst); err != nil {
		log.Error(err, "while deleting on apiserver")
		return err
	}
	return nil
}

func (r *AnchorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Maps an subnamespace to its anchor in the parent namespace.
	nsMapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			if a.Meta.GetAnnotations()[api.SubnamespaceOf] == "" {
				return nil
			}
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      a.Meta.GetName(),
					Namespace: a.Meta.GetAnnotations()[api.SubnamespaceOf],
				}},
			}
		})
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.SubnamespaceAnchor{}).
		Watches(&source.Channel{Source: r.Affected}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: nsMapFn}).
		Complete(r)
}
