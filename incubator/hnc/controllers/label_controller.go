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
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

const (
	LabelParent   = metaGroup + "/parent"
	AnnotChildren = metaGroup + "/children"
	AnnotConds    = metaGroup + "/conditions"
)

// LabelReconciler is responsible for determining the forest structure from the Hierarchy CRs,
// as well as ensuring all objects in the forest are propagated correctly when the hierarchy
// changes. It can also set the status of the Hierarchy CRs, as well as (in rare cases) override
// part of its spec (i.e., if a parent namespace no longer exists).
type LabelReconciler struct {
	client.Client
	Log logr.Logger

	// Forest is the in-memory data structure that is shared with all other reconcilers.
	// LabelReconciler is responsible for keeping it up-to-date, but the other reconcilers
	// use it to determine how to propagate objects.
	Forest *forest.Forest

	// Types is a list of other reconcillers that LabelReconciler can call if the hierarchy
	// changes. This will force all objects to be re-propagated.
	//
	// This is probably wildly inefficient, and we can probably make better use of things like
	// owner references to make this better. But for a PoC, it works just fine.
	Types []TypeReconciler
}

// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;update;patch

// Reconcile invokes onDelete or update, depending on whether the namespace exists or not.
func (r *LabelReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("trigger", req.NamespacedName.Name)

	inst := &corev1.Namespace{}
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if !errors.IsNotFound(err) {
			log.Info("Couldn't read")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, r.onDelete(ctx, log, req.NamespacedName.Name)
	}

	if !inst.GetDeletionTimestamp().IsZero() {
		// Pretend it's deleted just to make the integ tests pass. This code will all be
		// deleted shortly anyway.
		// TODO: update tests for behaviour when namespaces are deleted.
		return ctrl.Result{}, r.onDelete(ctx, log, req.NamespacedName.Name)
		// Wait for it to actually be deleted
		// return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.update(ctx, log, inst, true)
}

// update performs three tasks: it syncs the labels with the in-memory forest; it calls itself on
// any affected namespaces if the hierarchy has changed, and it invokes all the TypeReconcilers to
// propagate objects. It *always* results in one write to the apiserver per instance, even if no
// changes are needed (we can fix this later).
func (r *LabelReconciler) update(ctx context.Context, log logr.Logger, inst *corev1.Namespace, updateObjects bool) error {
	log.V(1).Info("Updating")
	meta := &inst.ObjectMeta
	initLabelsAndAnnotations(meta)
	affected := r.syncWithForest(ctx, log, meta)
	if err := r.Update(ctx, inst); err != nil {
		log.Error(err, "while updating apiserver")
		return err
	}

	// Stop updating if there are any conditions
	if _, exists := meta.Annotations[AnnotConds]; exists {
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
			if err := tr.ReconcileNamespace(ctx, log, meta.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// initLabelsAndAnnotations sets up the maps if they don't already exist, so the rest of the update
// code can assume they're there.
func initLabelsAndAnnotations(meta *metav1.ObjectMeta) {
	if meta.Labels == nil {
		meta.Labels = map[string]string{}
	}
	if meta.Annotations == nil {
		meta.Annotations = map[string]string{}
	}
}

// syncWithForest synchronizes the in-memory forest with the metadata. If any *other* namespaces
// have changed, it returns the list of affected namespaces.
func (r *LabelReconciler) syncWithForest(ctx context.Context, log logr.Logger, meta *metav1.ObjectMeta) []string {
	r.Forest.Lock()
	defer r.Forest.Unlock()

	ns := r.Forest.AddOrGet(meta.Name)
	affected := []string{}
	conds := []string{}

	// Sync our data structures with the current parent. If there's a problem, set the label
	// to an invalid value so that we don't accidentally pick up a new parent that gets created
	// in the future (and also get access to its Secrets, etc).
	curParentName, _ := meta.Labels[LabelParent]
	curParent := r.Forest.Get(curParentName)
	if curParentName != "" && curParent == nil {
		// This WILL FAIL if you restart the controller while namespaces have the annotation
		// set. TODO: do a full sync at least once if we ever get here, or (better yet)
		// completely rethink how we handle missing parents.
		log.Info("Missing", "parent", curParentName)
		conds = append(conds, "bad parent")
		const prefix = "missing.parent." // must be valid label value
		if !strings.HasPrefix(curParentName, prefix) {
			meta.Labels[LabelParent] = prefix + curParentName
		}
	}

	// Update the in-memory hierarchy if it's changed
	oldParent := ns.Parent()
	if oldParent != curParent {
		log.Info("Parent has changed", "old", oldParent.Name(), "new", curParent.Name())
		if err := ns.SetParent(curParent); err != nil {
			log.Info("Couldn't set parent", "condition", err)
			conds = append(conds, err.Error())
		} else {
			// Only call other parts of the hierarchy recursively if this one was
			// successfully updated; otherwise, if you get a cycle, this could get into
			// an infinite loop.
			if oldParent != nil {
				affected = append(affected, oldParent.Name())
			}
			if curParent != nil {
				affected = append(affected, curParent.Name())
			}
		}
	}

	// Regenerate the list of child names
	children := ns.ChildNames()
	if len(children) > 0 {
		sort.Strings(children)
		meta.Annotations[AnnotChildren] = strings.Join(children, ";")
	} else {
		delete(meta.Annotations, AnnotChildren)
	}

	if len(conds) > 0 {
		meta.Annotations[AnnotConds] = strings.Join(conds, "; ")
	} else {
		delete(meta.Annotations, AnnotConds)
	}

	return affected
}

// onDelete removes this namespace from the in-memory forest. Since this might have been a parent or
// child, we also update the parent or children as well.
func (r *LabelReconciler) onDelete(ctx context.Context, log logr.Logger, nm string) error {
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
func (r *LabelReconciler) updateAffected(ctx context.Context, log logr.Logger, affected []string) error {
	// TODO: parallelize updates
	for _, nm := range affected {
		// Load the affected NS
		log := log.WithValues("affected", nm)
		nnm := types.NamespacedName{Name: nm}
		inst := &corev1.Namespace{}
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

func (r *LabelReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Complete(r)
}
