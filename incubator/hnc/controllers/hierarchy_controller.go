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
	"sort"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
	Types []TypeReconciler
}

// TypeReconciler is a reconciler for a single Type (aka GVK). We should probably rename it to
// GVKReconciler.
type TypeReconciler interface {
	// ReconcileNamespace accepts the context, a logger, and the name of the namespace to be
	// reconciled.
	ReconcileNamespace(context.Context, logr.Logger, string) error
}

// +kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchies/status,verbs=get;update;patch

// Reconcile invokes onDelete or update, depending on whether the Hierarchy object exists or not.
func (r *HierarchyReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("trigger", req.NamespacedName.Namespace)

	inst := &tenancy.Hierarchy{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if !errors.IsNotFound(err) {
			log.Info("Couldn't read")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.onDelete(ctx, log, req.NamespacedName.Namespace)
	}

	if !inst.GetDeletionTimestamp().IsZero() {
		// Wait for it to actually be deleted
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.update(ctx, log, inst, true)
}

// update performs three tasks: it syncs the Hierarchy singleton with the in-memory forest; it calls
// itself on any affected namespaces if the hierarchy has changed, and it invokes all the
// TypeReconcilers to propagate objects. It *always* results in one write to the apiserver per
// instance, even if no changes are needed (we can fix this later).
func (r *HierarchyReconciler) update(ctx context.Context, log logr.Logger, inst *tenancy.Hierarchy, updateObjects bool) error {
	log.V(1).Info("Updating")
	affected := r.syncWithForest(ctx, log, inst)
	if err := r.Update(ctx, inst); err != nil {
		log.Error(err, "while updating apiserver")
		return err
	}

	// Stop updating if there are any conditions
	if len(inst.Status.Conditions) > 0 {
		log.Info("Early exit due to conditions")
		return nil
	}

	// Update any other namespaces that we believe may have been affected
	if err := r.updateAffected(ctx, log, affected); err != nil {
		return err
	}

	if updateObjects {
		// Update all the objects in this namespace. Not sure if it matters if we do this before or
		// after we update the other namespaces' hierarchy, but after kinda seems safer as a first
		// cut. On the other hand, if this is going to cause an error, we probably want to stop
		// _before_ we propagate the namespace changes to the rest of the hierarchy. TODO: revisit
		// and decide.
		for _, tr := range r.Types {
			if err := tr.ReconcileNamespace(ctx, log, inst.ObjectMeta.Namespace); err != nil {
				return err
			}
		}
	}

	return nil
}

// syncWithForest synchronizes the in-memory forest with the (in-memory) Hierarchy instance. If any
// *other* namespaces have changed, it returns the list of affected namespaces.
func (r *HierarchyReconciler) syncWithForest(ctx context.Context, log logr.Logger, inst *tenancy.Hierarchy) []string {
	r.Forest.Lock()
	defer r.Forest.Unlock()

	ns := r.Forest.AddOrGet(inst.ObjectMeta.Namespace)
	affected := []string{}
	conds := []tenancy.Condition{}

	// Sync our data structures with the current parent. The current parent might not exist (if, for
	// example, the hierarchy is being created as a result of `kubectl apply -f` on a directory); in
	// this case, just set a condition on the child, which will be removed once the parent exists.
	curParent := r.Forest.Get(inst.Spec.Parent)
	if inst.Spec.Parent != "" && curParent == nil {
		log.Info("Missing", "parent", curParent.Name())
		conds = append(conds, tenancy.Condition{Msg: "missing parent"})
	}

	// Update the in-memory hierarchy if it's changed
	oldParent := ns.Parent()
	if oldParent != curParent {
		log.Info("Parent has changed", "old", oldParent.Name(), "new", curParent.Name())
		if err := ns.SetParent(curParent); err != nil {
			log.Info("Couldn't set parent", "condition", err)
			conds = append(conds, tenancy.Condition{Msg: err.Error()})
		} else {
			// Only call other parts of the hierarchy recursively if this one was successfully updated;
			// otherwise, if you get a cycle, this could get into an infinite loop.
			if oldParent != nil {
				affected = append(affected, oldParent.Name())
			}
			if curParent != nil {
				affected = append(affected, curParent.Name())
			}
		}
	}

	// Update all other changed fields
	children := ns.ChildNames()
	sort.Strings(children)
	inst.Status.Children = children
	inst.Status.Conditions = conds

	return affected
}

// onDelete removes this namespace from the in-memory forest. Since this might have been a parent or
// child, we also update the parent or children as well.
func (r *HierarchyReconciler) onDelete(ctx context.Context, log logr.Logger, nm string) error {
	r.Forest.Lock()
	affected := r.Forest.Remove(nm)
	r.Forest.Unlock()
	log.Info("Removing from forest", "affected", affected)

	// Update any other namespaces that we believe may have been affected
	if err := r.updateAffected(ctx, log, affected); err != nil {
		return err
	}

	return nil
}

// updateAffected simply calls `update` on a list of namespaces we believe were affected by the
// current reconciliation.
func (r *HierarchyReconciler) updateAffected(ctx context.Context, log logr.Logger, affected []string) error {
	// TODO: parallelize updates
	for _, nm := range affected {
		// Load the affected NS
		log := log.WithValues("affected", nm)
		nnm := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
		inst := &tenancy.Hierarchy{}
		if err := r.Get(ctx, nnm, inst); err != nil {
			log.Error(err, "Couldn't read")
			return err
		}

		// Update it. Do *not* call the object reconcilers; if the affected namespace is
		// actually modified, the controller will naturally be called again, and the object
		// reconcilers will be run then.
		if err := r.update(ctx, log, inst, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *HierarchyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&tenancy.Hierarchy{}).
		Complete(r)
}
