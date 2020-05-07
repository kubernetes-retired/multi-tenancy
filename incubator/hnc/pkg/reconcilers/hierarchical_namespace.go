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
	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/metadata"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// HierarchicalNamespaceReconciler reconciles HierarchicalNamespace CRs to make sure
// all the hierarchical namespaces are properly maintained.
type HierarchicalNamespaceReconciler struct {
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
func (r *HierarchicalNamespaceReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("trigger", req.NamespacedName)
	log.Info("Reconciling HNS")

	// Get names of the hierarchical namespace and the current namespace.
	nm := req.Name
	pnm := req.Namespace

	// Get instance from apiserver. If the instance doesn't exist, do nothing and
	// early exist because HCR watches HNS instances and (if applicable) has
	// already updated the owner HC when this HNS was purged.
	inst, err := r.getInstance(ctx, pnm, nm)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Report "Forbidden" state and early exit if the namespace is not allowed to have subnamespaces
	// but has bypassed the webhook and successfully created the hns instance. Forbidden HNSes won't
	// have finalizers.
	// TODO refactor/split the EX map for 1) reconciler exclusion and 2) subnamespaces exclusion
	// purposes. See issue: https://github.com/kubernetes-sigs/multi-tenancy/issues/495
	if config.EX[pnm] {
		inst.Status.State = api.Forbidden
		return ctrl.Result{}, r.writeInstance(ctx, log, inst)
	}

	// Get the owned child namespace instance. If it doesn't exist, initialize one.
	cnsInst, err := r.getNamespace(ctx, nm)
	if err != nil {
		return ctrl.Result{}, err
	}

	if deleting, err := r.onDeleting(ctx, log, inst, cnsInst); deleting {
		return ctrl.Result{}, err
	}

	// Update the state. If the owned child namespace doesn't exist, create it.
	// (This is how a subnamespace is created through a CR)
	r.updateState(log, inst, cnsInst)
	if inst.Status.State == api.Missing {
		if err := r.writeNamespace(ctx, log, nm, pnm); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Add finalizers on all non-forbidden HNSes to ensure it's not deleted until
	// after the owned child namespace is deleted.
	inst.ObjectMeta.Finalizers = []string{api.MetaGroup}
	return ctrl.Result{}, r.writeInstance(ctx, log, inst)
}

func (r *HierarchicalNamespaceReconciler) onDeleting(ctx context.Context, log logr.Logger, inst *api.HierarchicalNamespace, cnsInst *corev1.Namespace) (deleting bool, err error) {
	// Early exit and continue reconciliation if the instance is not being deleted.
	if inst.DeletionTimestamp.IsZero() {
		return false, nil
	}

	cnm := inst.Name
	log.Info("The HNS instance is being deleted.")
	switch {
	case len(inst.ObjectMeta.Finalizers) == 0:
		// We've finished process this, nothing to do.
		log.Info("Do nothing since the finalizers are already gone.")
		return true, nil
	case cnsInst.DeletionTimestamp.IsZero() && r.allowsCascadingDelete(cnm):
		// The owned namespace is not being deleted but it allows cascadingDelete.
		// Delete the owned namespace.
		log.Info("The subnamespace is not being deleted but it allows cascading deletion.")
		return true, r.deleteNamespace(ctx, log, cnsInst)
	case r.removeFinalizers(log, inst, cnsInst):
		log.Info("Remove the finalizers.")
		return true, r.writeInstance(ctx, log, inst)
	default:
		// The owned namespace is being deleted. Do nothing in this Reconcile().
		// Wait until it's purged to remove the finalizers in another Reconcile().
		log.Info("Do nothing since the owned namespace is still being deleted (not purged yet).")
		return true, nil
	}
}

func (r *HierarchicalNamespaceReconciler) removeFinalizers(log logr.Logger, inst *api.HierarchicalNamespace, cnsInst *corev1.Namespace) bool {
	pnm := inst.Namespace
	cnm := inst.Name
	switch {
	case cnsInst.Name == "":
		log.Info("The owned namespace is already purged.")
	case cnsInst.GetAnnotations()[api.AnnotationOwner] != pnm:
		log.Info("The subnamespace is not owned by this namespace.")
	case cnsInst.DeletionTimestamp.IsZero() && !r.allowsCascadingDelete(cnm):
		log.Info("The subnamespace is not being deleted and it doesn't allow cascading deletion.")
	default:
		return false
	}

	inst.ObjectMeta.Finalizers = nil
	return true
}

func (r *HierarchicalNamespaceReconciler) updateState(log logr.Logger, inst *api.HierarchicalNamespace, cnsInst *corev1.Namespace) {
	nm := inst.Name
	pnm := inst.Namespace
	switch {
	case cnsInst.Name == "":
		log.Info("The owned subnamespace does not exist", "subnamespace", nm)
		inst.Status.State = api.Missing
	case cnsInst.Annotations[api.AnnotationOwner] != pnm:
		log.Info("The owner annotation of the subnamespace doesn't match the owner", "annotation", cnsInst.Annotations[api.AnnotationOwner])
		inst.Status.State = api.Conflict
	default:
		log.Info("The subnamespace has the correct owner annotation", "annotation", cnsInst.Annotations[api.AnnotationOwner])
		inst.Status.State = api.Ok
	}
}

// It enqueues a hierarchicalNamespace instance for later reconciliation. This occurs in a goroutine
// so the caller doesn't block; since the reconciler is never garbage-collected, this is safe.
func (r *HierarchicalNamespaceReconciler) enqueue(log logr.Logger, nm, pnm, reason string) {
	go func() {
		// The watch handler doesn't care about anything except the metadata.
		inst := &api.HierarchicalNamespace{}
		inst.ObjectMeta.Name = nm
		inst.ObjectMeta.Namespace = pnm
		log.Info("Enqueuing for reconciliation", "affected", pnm+"/"+nm, "reason", reason)
		r.Affected <- event.GenericEvent{Meta: inst}
	}()
}

func (r *HierarchicalNamespaceReconciler) getInstance(ctx context.Context, pnm, nm string) (*api.HierarchicalNamespace, error) {
	nsn := types.NamespacedName{Namespace: pnm, Name: nm}
	inst := &api.HierarchicalNamespace{}
	if err := r.Get(ctx, nsn, inst); err != nil {
		return nil, err
	}
	return inst, nil
}

func (r *HierarchicalNamespaceReconciler) writeInstance(ctx context.Context, log logr.Logger, inst *api.HierarchicalNamespace) error {
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
func (r *HierarchicalNamespaceReconciler) getNamespace(ctx context.Context, nm string) (*corev1.Namespace, error) {
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

func (r *HierarchicalNamespaceReconciler) writeNamespace(ctx context.Context, log logr.Logger, nm, pnm string) error {
	inst := &corev1.Namespace{}
	inst.ObjectMeta.Name = nm
	metadata.SetAnnotation(inst, api.AnnotationOwner, pnm)

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

func (r *HierarchicalNamespaceReconciler) deleteNamespace(ctx context.Context, log logr.Logger, inst *corev1.Namespace) error {
	log.Info("Deleting namespace on apiserver")
	if err := r.Delete(ctx, inst); err != nil {
		log.Error(err, "while deleting on apiserver")
		return err
	}
	return nil
}

func (r *HierarchicalNamespaceReconciler) allowsCascadingDelete(nm string) bool {
	r.forest.Lock()
	defer r.forest.Unlock()

	ns := r.forest.Get(nm)
	return ns.AllowsCascadingDelete()
}

func (r *HierarchicalNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Maps an owned namespace to its HNS instance in the owner namespace.
	nsMapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			if a.Meta.GetAnnotations()[api.AnnotationOwner] == "" {
				return nil
			}
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      a.Meta.GetName(),
					Namespace: a.Meta.GetAnnotations()[api.AnnotationOwner],
				}},
			}
		})
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.HierarchicalNamespace{}).
		Watches(&source.Channel{Source: r.Affected}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &corev1.Namespace{}}, &handler.EnqueueRequestsFromMapFunc{ToRequests: nsMapFn}).
		Complete(r)
}
