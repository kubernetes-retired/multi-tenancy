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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

const (
	metaGroup          = "hnc.x-k8s.io"
	labelInheritedFrom = metaGroup + "/inheritedFrom"
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
}

// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch;create;update;patch;delete

func (r *ObjectReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	resp := ctrl.Result{}
	ctx := context.Background()
	log := r.Log.WithValues("trigger", req.NamespacedName)

	// Read the object; if it's missing, assume it's been deleted. If we miss a deletion,
	// hopefully SyncNamespace will pick up on it.
	inst := &unstructured.Unstructured{}
	inst.SetGroupVersionKind(r.GVK)
	if err := r.Get(ctx, req.NamespacedName, inst); err != nil {
		if !errors.IsNotFound(err) {
			log.Info("Couldn't read")
			return resp, err
		}

		return resp, r.onDelete(ctx, log, req.NamespacedName)
	}

	// If it's about to be deleted, do nothing, just wait for it to be actually deleted.
	if !inst.GetDeletionTimestamp().IsZero() {
		log.Info("Will soon be deleted")
		return resp, nil
	}

	// Handle any changes.
	return resp, r.update(ctx, log, inst)
}

// SyncNamespace can be called manually by the HierarchyReconciler when the hierarchy changes
// to force a full refresh of all objects (of the given GVK) in this namespace. It's probably wildly
// slow and inefficient.
func (r *ObjectReconciler) SyncNamespace(ctx context.Context, log logr.Logger, ns string) error {
	log = log.WithValues("gvk", r.GVK)
	ul := &unstructured.UnstructuredList{}
	ul.SetGroupVersionKind(r.GVK)
	if err := r.List(ctx, ul, client.InNamespace(ns)); err != nil {
		log.Error(err, "Couldn't list objects")
		return err
	}
	// TODO: parallelize.
	for _, inst := range ul.Items {
		if err := r.update(ctx, log.WithValues("object", inst.GetName()), &inst); err != nil {
			return err
		}
	}
	return nil
}

// update deletes this object if it's an obsolete copy, and otherwise ensures it's been propagated
// to any child namespaces.
func (r *ObjectReconciler) update(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) error {
	// Make sure this object is actually supposed to exist. If not, delete it. This should
	// trigger one more reconciliation cycle and we'll get the children then.
	if deleted, err := r.deleteObsolete(ctx, log, inst); deleted || err != nil {
		return err
	}

	// Make sure this gets propagated to any children that exist
	return r.propagate(ctx, log, inst)
}

// deleteObsolete checks to see if the given object should not exist any more. It returns true if
// it deleted the object and false otherwise.
func (r *ObjectReconciler) deleteObsolete(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) (bool, error) {
	// Find what we expect the source to be. If there's no source, *this*
	// is the source and there's nothing to do.
	labels := inst.GetLabels()
	if labels == nil {
		// this cannot be a copy
		return false, nil
	}
	inheritedFrom, _ := labels[labelInheritedFrom]
	if inheritedFrom == "" {
		// this isn't a copy
		return false, nil
	}

	// Read the source. If we find it, and the namespace is still an ancestor, no problem.
	srcNm := types.NamespacedName{Namespace: inheritedFrom, Name: inst.GetName()}
	src := &unstructured.Unstructured{}
	src.SetGroupVersionKind(inst.GroupVersionKind())
	err := r.Get(ctx, srcNm, src)
	if err == nil {
		// The object exists. Is it still in an ancestor namespace?
		if yes, err := r.isAncestor(inst.GetNamespace(), inheritedFrom); err != nil {
			// This can fail if the names are bad. For now, take no action (ie don't
			// delete). TODO: revisit this.
			log.Error(err, "deleteObsolete")
			return false, err
		} else if yes {
			// We found the object in an ancestor namespace; no need to delete it.
			return false, nil
		}
		log.Info("Will delete because the source namespace is no longer an ancestor", "inheritedFrom", inheritedFrom)
	} else if errors.IsNotFound(err) {
		log.Info("Will delete because the source object no longer exists", "inheritedFrom", inheritedFrom)
	} else {
		log.Info("Couldn't read", "source", srcNm)
		return false, err
	}

	// Delete this instance.
	if err := r.Delete(ctx, inst); err != nil {
		log.Error(err, "Couldn't delete after source was missing")
		return false, err
	}

	return true, nil
}

// propagate copies the object to the child namespaces. No need to do this recursively as the
// controller will automatically pick up the changes in the child and call this method again.
//
// TODO: parallelize?
func (r *ObjectReconciler) propagate(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) error {
	if r.isExcluded(log, inst) {
		return nil
	}
	parent := inst.GetNamespace()
	for _, child := range r.getChildNamespaces(inst.GetNamespace()) {
		// Create an in-memory copy with the appropriate namespace.
		copied := copyObject(inst, child)

		// If the label to the source namespace is missing, then the object we're copying
		// must be the original, so point the label to this namespace.
		labels := copied.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		if _, exists := labels[labelInheritedFrom]; !exists {
			labels[labelInheritedFrom] = parent
			copied.SetLabels(labels)
		}

		// Push to the apiserver
		log.Info("Propagating", "dst", child, "origin", labels[labelInheritedFrom])
		err := r.Update(ctx, copied)
		if err != nil && errors.IsNotFound(err) {
			err = r.Create(ctx, copied)
		}
		if err != nil {
			log.Error(err, "Couldn't propagate", "copy", copied)
			return err
		}
	}

	return nil
}

// onDelete explicitly deletes the children that were propagated from this object.
func (r *ObjectReconciler) onDelete(ctx context.Context, log logr.Logger, nnm types.NamespacedName) error {
	// Delete the children. This *should* trigger the reconciler to run on each of them in turn
	// so no need to do this recursively. TODO: maybe do a search on the label value instead?
	// Parallelize?
	for _, child := range r.getChildNamespaces(nnm.Namespace) {
		// Try to find the copied objects, if they exist
		copiedNnm := types.NamespacedName{Namespace: child, Name: nnm.Name}
		copied := &unstructured.Unstructured{}
		copied.SetGroupVersionKind(r.GVK)
		err := r.Get(ctx, copiedNnm, copied)

		// Is it already gone?
		if errors.IsNotFound(err) {
			continue
		}
		// Some other error?
		if err != nil {
			log.Error(err, "Couldn't read copied object that needs to be deleted", "name", copiedNnm)
			return err
		}

		// TODO: double-check the label - or maybe just call deleteObsolete?

		// Delete the copy
		log.Info("Deleting", "propagated", copiedNnm)
		if err := r.Delete(ctx, copied); err != nil {
			log.Error(err, "Coudln't delete", "copy", copied)
			return err
		}
	}

	return nil
}

func copyObject(inst *unstructured.Unstructured, ns string) *unstructured.Unstructured {
	copied := inst.DeepCopy()

	// Clear all irrelevant fields. cf https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.15/#objectmeta-v1-meta
	copied.SetCreationTimestamp(metav1.Time{})
	copied.SetDeletionGracePeriodSeconds(nil)
	copied.SetDeletionTimestamp(nil)
	copied.SetFinalizers(nil) // TODO: double-check this is the right thing to do?
	copied.SetGenerateName("")
	copied.SetGeneration(0)
	copied.SetInitializers(nil) // TODO: is this correct?
	// TODO: what about managedFields?
	// TODO: use ownerReferences instead?
	copied.SetResourceVersion("")
	copied.SetSelfLink("")
	copied.SetUID("")

	copied.SetNamespace(ns)
	return copied
}

func (r *ObjectReconciler) isAncestor(nsNm, otherNm string) (bool, error) {
	r.Forest.Lock()
	defer r.Forest.Unlock()
	ns := r.Forest.Get(nsNm)
	if !ns.Exists() {
		return false, fmt.Errorf("unknown namespace %q", nsNm)
	}
	nsSrc := r.Forest.Get(otherNm)
	if !nsSrc.Exists() {
		return false, fmt.Errorf("unknown namespace %q", otherNm)
	}
	return ns.IsAncestor(nsSrc), nil
}

func (r *ObjectReconciler) getChildNamespaces(nm string) []string {
	r.Forest.Lock()
	defer r.Forest.Unlock()
	ns := r.Forest.Get(nm)
	children := []string{}
	if ns != nil {
		children = ns.ChildNames()
	}
	return children
}

// isExcluded returns true if the object shouldn't be handled by the HNC. Eventually, this may be
// user-configurable, but right now it's only used for Service Account token secrets.
func (r *ObjectReconciler) isExcluded(log logr.Logger, inst *unstructured.Unstructured) bool {
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

func (r *ObjectReconciler) SetupWithManager(mgr ctrl.Manager) error {
	target := &unstructured.Unstructured{}
	target.SetGroupVersionKind(r.GVK)
	return ctrl.NewControllerManagedBy(mgr).For(target).Complete(r)
}
