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
	annotationModified = metaGroup + "/modified"
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
	// Make sure this object is correct and supposed to propagate. If not, delete it.
	srcInst, err := r.getSourceInst(ctx, log, inst)
	if err != nil {
		return err
	}

	dests, good := r.syncWithForest(ctx, log, inst, srcInst)
	if !good {
		return r.Delete(ctx, inst)
	}

	// Make sure this gets propagated to any children that exist
	return r.propagate(ctx, log, inst, dests)
}

// syncWithForest syncs the object instance with the in-memory forest. This is the only place we
// should lock the forest, so this fn needs to return everything relevant for the rest of the
// reconciler. This fn shouldn't contact the apiserver since that's a slow operation and everything
// will block on the lock being held. It returns true if the object instance is good and shouldn't
// be deleted. It will also return a list of destinations for propagation.
func (r *ObjectReconciler) syncWithForest(ctx context.Context, log logr.Logger, inst, srcInst *unstructured.Unstructured) ([]string, bool) {
	r.Forest.Lock()
	defer r.Forest.Unlock()

	// If srcInst isn't an ancestor of inst anymore, it should be deleted
	if !r.hasCorrectAncestry(ctx, log, inst, srcInst) {
		return nil, false
	}

	// If the object has been modified, alert the users and don't propagate it further
	if !r.isUnmodified(ctx, log, inst, srcInst) {
		// TODO add object condition here to replace the setAnnotation
		log.Info("Marking as modified from the source", "inheritedFrom", srcInst.GetNamespace())
		// Mark as modified if the copied fields don't match.
		setAnnotation(inst, annotationModified, "true")
		// TODO it's wrong to call apiserver in forest lock, will be replaced soon.
		err := r.Update(ctx, inst)
		if err != nil {
			log.Error(err, "Couldn't add modified annotation")
		}
		return nil, true
	}

	// The object looks good; it should be propagated to its namespaces' children
	return r.Forest.Get(inst.GetNamespace()).ChildNames(), true
}

// hasCorrectAncestry checks to see if the given object has correct ancestry. It returns false
// if the source namespace is no longer an ancestor and the object should be deleted.
func (r *ObjectReconciler) hasCorrectAncestry(ctx context.Context, log logr.Logger, inst, src *unstructured.Unstructured) bool {
	if src == nil {
		log.Info("Will delete because the source instance no longer exists", "inheritedFrom", getSourceNS(inst))
		return false
	}

	if inst == src {
		return true
	}

	// The source exists. Is it still in an ancestor namespace?
	curNS := r.Forest.Get(inst.GetNamespace())
	srcNS := r.Forest.Get(src.GetNamespace())
	if !curNS.IsAncestor(srcNS) {
		log.Info("Will delete because the source namespace is no longer an ancestor", "inheritedFrom", srcNS)
		return false
	}

	return true
}

// isUnmodified checks to see if this object is the same as the source.
func (r *ObjectReconciler) isUnmodified(ctx context.Context, log logr.Logger, inst, src *unstructured.Unstructured) bool {
	if src == inst {
		return true
	}

	instCopied := copyObject(inst)
	// The only difference could be namespace and labelInheritedFrom label.
	// Make the namespaces and inheritedFrom labels the same before comparison.
	instCopied.SetNamespace(src.GetNamespace())
	setLabel(instCopied, labelInheritedFrom, "")
	srcCopied := copyObject(src)
	setLabel(srcCopied, labelInheritedFrom, "")
	return reflect.DeepEqual(instCopied, srcCopied)
}

// getSourceInst gets source instance of this one. It returns itself if it's the source. It returns
// nil if the source instance no longer exists in the source namespace or there's an error.
func (r *ObjectReconciler) getSourceInst(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	srcNS := getSourceNS(inst)
	if srcNS == "" {
		// this is the src itself
		return inst, nil
	}

	// Get the source instance from source namespace
	srcNm := types.NamespacedName{Namespace: srcNS, Name: inst.GetName()}
	src := &unstructured.Unstructured{}
	src.SetGroupVersionKind(inst.GroupVersionKind())
	err := r.Get(ctx, srcNm, src)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("The source object no longer exists", "inheritedFrom", srcNS)
			return nil, nil
		} else {
			log.Error(err, "Couldn't read", "source", srcNm)
			return nil, err
		}
	}

	return src, nil
}

// getSourceNS gets source namespace if it's set in "labelInheritedFrom" label.
// It returns an empty string if this instance is the source
func getSourceNS(inst *unstructured.Unstructured) string {
	labels := inst.GetLabels()
	if labels == nil {
		// this cannot be a copy
		return ""
	}
	inheritedFrom, _ := labels[labelInheritedFrom]
	return inheritedFrom
}

// propagate copies the object to the child namespaces. No need to do this recursively as the
// controller will automatically pick up the changes in the child and call this method again.
//
// TODO: parallelize?
func (r *ObjectReconciler) propagate(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured, dests []string) error {
	if r.isExcluded(log, inst) {
		return nil
	}
	parent := inst.GetNamespace()
	for _, child := range dests {
		// Create an in-memory copy with the appropriate namespace.
		copied := copyObject(inst)
		copied.SetNamespace(child)

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

func copyObject(inst *unstructured.Unstructured) *unstructured.Unstructured {
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

	return copied
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

func setAnnotation(inst *unstructured.Unstructured, annotation string, value string) {
	annotations := inst.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[annotation] = value
	inst.SetAnnotations(annotations)
}

type apiInstances interface {
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
}

func setLabel(inst apiInstances, label string, value string) {
	labels := inst.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[label] = value
	inst.SetLabels(labels)
}
