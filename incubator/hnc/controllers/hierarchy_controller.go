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
	"reflect"
	"sort"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
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
	Types []NamespaceSyncer
}

// NamespaceSyncer syncs various aspects of a namespace. The HierarchyReconciler both implements
// it (so it can be called by NamespaceSyncer) and uses it (to sync the objects in the
// namespace).
type NamespaceSyncer interface {
	SyncNamespace(context.Context, logr.Logger, string) error
}

// +kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=hnc.x-k8s.io,resources=hierarchies/status,verbs=get;update;patch

// Reconcile simply calls SyncNamespace, which can also be called if a namespace is created or
// deleted.
func (r *HierarchyReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	nnm := req.NamespacedName
	log := r.Log.WithValues("trigger", req.NamespacedName)
	return ctrl.Result{}, r.SyncNamespace(ctx, log, nnm.Namespace)
}

func (r *HierarchyReconciler) SyncNamespace(ctx context.Context, log logr.Logger, nm string) error {
	update := true
	missing := false

	// Get the hierarchy itself.
	nnm := types.NamespacedName{Namespace: nm, Name: tenancy.Singleton}
	inst := &tenancy.Hierarchy{}
	if err := r.Get(ctx, nnm, inst); err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "Couldn't read singleton", "ns", nm)
			return err
		}

		// It doesn't exist - initialize it to a sane initial value.
		log.V(1).Info("Missing singleton", "ns", nm)
		missing = true
		inst.ObjectMeta.Name = tenancy.Singleton
		inst.ObjectMeta.Namespace = nm
	}

	// If the hierarchy object exists but is being deleted, we won't update it when we're finished
	// syncing (we should sync our internal data structure anyway just in case something's changed).
	if !inst.GetDeletionTimestamp().IsZero() {
		log.Info("Singleton is being deleted", "ns", nm)
		update = false
	}

	// If it's missing, check to see if the namespace is missing as well. If so, we need to remove
	// this namespace from the forest.
	if missing {
		ns := &corev1.Namespace{}
		nsnnm := types.NamespacedName{Name: nm}
		if err := r.Get(ctx, nsnnm, ns); err != nil {
			if !errors.IsNotFound(err) {
				log.Error(err, "Couldn't read namespace", "ns", nm)
				return err
			}

			// The namespace is gone. Remove all traces of the namespace from  our data structures.
			log.Info("Namespace was deleted", "ns", nm)
			return r.onDelete(ctx, log, nm)
		}

		// If the namespace is being deleted, we should still sync any changes that we might have missed
		// (ie, maybe the parent used to be set?) but then don't update anything.
		if !ns.GetDeletionTimestamp().IsZero() {
			log.Info("Namespace is being deleted", "ns", nm)
			update = false
		}
	}

	// Sync the tree. If `ok` is false, that means that there are conditions on this namespace and we
	// shouldn't continue to sync the objects.
	if ok, err := r.updateTree(ctx, log, inst, update); !ok || err != nil {
		return err
	}

	if update {
		// Update all the objects in this namespace. We have to do this at least *after* the tree is
		// updated, because if we don't, we could incorrectly think we've propagated the wrong objects
		// from our ancestors, or are propagating the wrong objects to our descendants.
		return r.updateObjects(ctx, log, inst.ObjectMeta.Namespace)
	}

	return nil
}

// updateTree syncs the Hierarchy singleton with the in-memory forest (writing back to the apiserver
// if necessary and requested) and calls itself on any affected namespaces if the hierarchy has
// changed. It returns false if there are conditions on this namespace, which indicates that any
// changes from this namespace should not be further propagated.
//
// TODO: store the conditions in the in-memory forest so that object propagation can be disabled if
// there's a problem on the namespace.
func (r *HierarchyReconciler) updateTree(ctx context.Context, log logr.Logger, inst *tenancy.Hierarchy, update bool) (bool, error) {
	// Update the in-memory data structures
	orig := inst.DeepCopy()
	affected := r.syncWithForest(ctx, log, inst)

	// If appropriate, write back. We only write if there's been a *change* in the data structure, and
	// if the caller wants us to update it (ie, if the namespace isn't being deleted).
	if update && !reflect.DeepEqual(orig, inst) {
		if inst.CreationTimestamp.IsZero() {
			log.Info("Creating singleton on apiserver", "ns", inst.ObjectMeta.Namespace)
			if err := r.Create(ctx, inst); err != nil {
				log.Error(err, "while creating on apiserver")
				return false, err
			}
		} else {
			log.Info("Updating singleton on apiserver", "ns", inst.ObjectMeta.Namespace)
			if err := r.Update(ctx, inst); err != nil {
				log.Error(err, "while updating apiserver")
				return false, err
			}
		}
	}

	// Stop updating if there are any conditions
	if len(inst.Status.Conditions) > 0 {
		log.Info("Early exit due to conditions")
		return false, nil
	}

	// Update any other namespaces that we believe may have been affected
	if err := r.updateAffected(ctx, log, affected); err != nil {
		return false, err
	}

	return true, nil
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
	//
	// TODO: detect when a missing parent gets added so we can remove this condition.
	curParent := r.Forest.Get(inst.Spec.Parent)
	if inst.Spec.Parent != "" && curParent == nil {
		log.Info("Couldn't set parent", "reason", "missing", "parent", inst.Spec.Parent)
		conds = append(conds, tenancy.Condition{Msg: "missing parent"})
	}

	// Update the in-memory hierarchy if it's changed
	oldParent := ns.Parent()
	if oldParent != curParent {
		log.Info("Parent has changed", "old", oldParent.Name(), "new", curParent.Name())
		if err := ns.SetParent(curParent); err != nil {
			log.Info("Couldn't set parent", "reason", err, "parent", inst.Spec.Parent)
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

	// Update all other changed fields. If they're empty, set them to nil so that they compare
	// properly.
	children := ns.ChildNames()
	if len(children) > 0 {
		sort.Strings(children)
		inst.Status.Children = children
	} else {
		inst.Status.Children = nil
	}
	if len(conds) > 0 {
		inst.Status.Conditions = conds
	} else {
		inst.Status.Conditions = nil
	}

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

// updateAffected simply calls `SyncNamespace` on a list of namespaces we believe were affected by
// the current reconciliation.
func (r *HierarchyReconciler) updateAffected(ctx context.Context, log logr.Logger, affected []string) error {
	// TODO: parallelize updates
	for _, nm := range affected {
		if err := r.SyncNamespace(ctx, log, nm); err != nil {
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
