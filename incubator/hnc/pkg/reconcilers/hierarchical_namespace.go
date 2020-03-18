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
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// HierarchicalNamespaceReconciler reconciles HierarchicalNamespace CRs to make sure
// all the hierarchical namespaces are properly maintained.
type HierarchicalNamespaceReconciler struct {
	client.Client
	Log logr.Logger

	forest *forest.Forest
	hcr    *HierarchyConfigReconciler

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

	// Get instance from apiserver. If the instance doesn't exist, we don't want to reconcile
	// it since it may trigger the HC reconciler to recreate the namespace that was just deleted.
	// TODO expand on this to check the owner's hc.spec.allowCascadingDelete. If it's set to
	//  true, we still want to reconcile the hns instance. See issue:
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/501
	inst, err := r.getInstance(ctx, pnm, nm)
	if err != nil {
		if errors.IsNotFound(err) {
			// If the instance doesn't exist, return nil to prevent a retry.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Report "Forbidden" state and early exist if the namespace is not allowed to self-serve
	// namespaces but has bypassed the webhook and successfully created the hns instance.
	// TODO refactor/split the EX map for 1) reconciler exclusion and 2) self-serve not allowed
	//  purposes. See issue: https://github.com/kubernetes-sigs/multi-tenancy/issues/495
	if EX[pnm] {
		inst.Status.State = api.Forbidden
		return ctrl.Result{}, r.writeInstance(ctx, log, inst)
	}

	// Get the self-serve namespace's hierarchyConfig and namespace instances.
	hcInst, nsInst, err := r.hcr.GetInstances(ctx, log, nm)
	if err != nil {
		return ctrl.Result{}, err
	}

	r.syncWithForest(log, inst, nsInst, hcInst)

	// TODO report the "SubnamespaceConflict" in the HNS reconciliation. See issue:
	//  https://github.com/kubernetes-sigs/multi-tenancy/issues/490

	return ctrl.Result{}, r.writeInstance(ctx, log, inst)
}

// HierarchicalNamespace (HNS) is synced with the in-memory forest, the HierarchyConfig and
// the namespace instances to update its HNS state. This will be the only place the "Owner"
// field in the forest is set. Therefore, the "Owner" field can be used in the forest to get
// all the HNS objects of a namespace.
func (r *HierarchicalNamespaceReconciler) syncWithForest(log logr.Logger, inst *api.HierarchicalNamespace, nsInst *corev1.Namespace, hcInst *api.HierarchyConfiguration) {
	r.forest.Lock()
	defer r.forest.Unlock()

	// Names of the hierarchical namespace and the current namespace.
	nm := inst.Name
	pnm := inst.Namespace

	// Get the namespace instance in memory.
	ns := r.forest.Get(nm)

	switch {
	case nsInst.Name == "":
		// If the real namespace instance doesn't exist yet, update forest and enqueue the namespace
		// to HierarchyConfig reconciler.
		log.Info("The self-serve subnamespace does not exist", "namespace", nm)
		r.syncMissing(log, inst, ns)
	case nsInst.Annotations[api.AnnotationOwner] != pnm:
		// If the namespace is not a self-serve namespace or it's a self-serve namespace of another
		// namespace. Report the conflict.
		log.Info("The owner annotation of the subnamespace doesn't match the owner", "annotation", nsInst.Annotations[api.AnnotationOwner])
		inst.Status.State = api.Conflict
	default:
		log.Info("The subnamespace has the correct owner annotation", "annotation", nsInst.Annotations[api.AnnotationOwner])
		r.syncExisting(log, inst, ns, hcInst, nsInst)
	}
}

func (r *HierarchicalNamespaceReconciler) syncMissing(log logr.Logger, inst *api.HierarchicalNamespace, ns *forest.Namespace) {
	pnm := inst.Namespace
	nm := inst.Name

	// Set the HNS state to "Missing" because the subnamespace doesn't exist.
	inst.Status.State = api.Missing

	// Set the "Owner" in the forest of the hierarchical namespace to the current namespace.
	log.Info("Setting the subnamespace's owner in the forest", "owner", pnm, "namespace", nm)
	ns.Owner = pnm

	// Enqueue the not-yet existent self-serve subnamespace. The HierarchyConfig reconciler will
	// create the namespace and the HierarchyConfig instances on apiserver accordingly.
	r.hcr.enqueueAffected(log, "new subnamespace", nm)
}

// syncExisting syncs the existing subnamespace with its owner namespace. It updates the HNS state
// to "Ok" or "Conflict" according to the hierarchy.
func (r *HierarchicalNamespaceReconciler) syncExisting(log logr.Logger, inst *api.HierarchicalNamespace, ns *forest.Namespace, hcInst *api.HierarchyConfiguration, nsInst *corev1.Namespace) {
	pnm := inst.Namespace
	nm := inst.Name

	switch hcInst.Spec.Parent {
	case "":
		log.Info("Parent is not set", "namespace", nm)
		// This case is rare. It means the namespace is created with the right annotation but
		// no HC. This could be a transient state before HC reconciler finishes creating the HC
		// or a human manually created the namespace with the right annotation but no HC.
		// Both cases meant to create this namespace as HNS.
		log.Info("Setting the subnamespace's owner in the forest", "owner", pnm, "namespace", nm)
		ns.Owner = pnm
		r.hcr.enqueueAffected(log, "updated subnamespace", nm)

		// We will set it as "Conflict" though it's just a short transient state. Once the hc is
		// reconciled, this HNS will be enqueued and then set the state to "Ok".
		inst.Status.State = api.Conflict
	case pnm:
		log.Info("Setting the HierarchicalNamespace state to Ok")
		inst.Status.State = api.Ok
	default:
		log.Info("Self-serve subnamespace is already owned by another parent", "child", nm, "intendedParent", pnm, "actualParent", hcInst.Spec.Parent)
		inst.Status.State = api.Conflict
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

func (r *HierarchicalNamespaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.HierarchicalNamespace{}).
		Watches(&source.Channel{Source: r.Affected}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}
