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
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// ObjectReconciler reconciles generic propagated objects. You must create one for each
// group/version/kind that needs to be propagated and set its `GVK` field appropriately.
type ObjectReconciler struct {
	client.Client
	Log logr.Logger

	// Forest is the in-memory forest managed by the HierarchyReconciler.
	Forest *forest.Forest

	// HierarchyReconciler is for reconciling namespaces when object conditions are detected.
	HierarchyReconciler *HierarchyReconciler

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
	// If for some reason this has been called on an object that isn't namespaced, let's generate some
	// logspam!
	if inst.GetNamespace() == "" {
		for i := 0; i < 100; i++ {
			log.Info("Non-namespaced object!!!")
		}
		return nil
	}

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

	log.Info("Checking")

	// If srcInst isn't an ancestor of inst anymore, it should be deleted
	if !r.hasCorrectAncestry(ctx, log, inst, srcInst) {
		return nil, false
	}

	// If any ancestor has ObjectOverridden condition, don't propagate it.
	if r.hasOverriddenAncestry(ctx, log, inst, srcInst) {
		return nil, true
	}

	// If the object has been overridden, alert the users and don't propagate it further.
	if r.isOverridden(ctx, log, inst, srcInst) {
		return nil, true
	}

	// TODO set ObjectAncestorOverridden condition on the descendants whose expected objects are
	//  not propagated because an ancestor object is overridden.

	dsts := []string{}
	for _, child := range r.Forest.Get(inst.GetNamespace()).ChildNames() {
		cns := r.Forest.Get(child)
		srcKey := affectedObjectKey(srcInst)
		if !cns.HasKeyedCondition(srcKey, api.ObjectOverridden) {
			log.Info("Child not overridden", "child", child)
			dsts = append(dsts, child)
		} else {
			log.Info("Child overridden!", "child", child)
		}
	}
	// The object looks good; it should be propagated to its namespaces' children
	return dsts, true
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

func (r *ObjectReconciler) hasOverriddenAncestry(ctx context.Context, log logr.Logger, inst, src *unstructured.Unstructured) bool {
	if inst == src {
		return false
	}

	srcKey := affectedObjectKey(src)
	sns := r.Forest.Get(src.GetNamespace())
	if !sns.HasConditionCode(api.ObjectDescendantOverridden) {
		// Early exit if the source doesn't have ObjectDescendantOverridden condition.
		return false
	}

	// Find if any ancestor (including itself) has ObjectOverridden condition.
	ans := r.Forest.Get(inst.GetNamespace()).Parent()
	for ans != sns {
		curKey := replaceKeyNamespace(srcKey, ans.Name())
		if ans.HasKeyedCondition(srcKey, api.ObjectOverridden) {
			log.Info("Ancestor modified", "modified", curKey)
			return true
		}
		ans = ans.Parent()
	}

	return false
}

// isOverridden returns true if the instance is overridden from the source. It will set/unset accordingly
// object conditions in the forest and enqueue affected namespaces to HierarchyReconciler for updating
// the conditions with the API server.
func (r *ObjectReconciler) isOverridden(ctx context.Context, log logr.Logger, inst, src *unstructured.Unstructured) bool {
	ak := affectedObjectKey(inst)
	sak := affectedObjectKey(src)
	msgObj := fmt.Sprintf("object overridden from source %s", sak)
	msgDsc := fmt.Sprintf("descendent object %s overridden", ak)
	nnm := inst.GetNamespace()
	snnm := src.GetNamespace()
	ns := r.Forest.Get(nnm)
	sns := r.Forest.Get(snnm)

	overridden := true
	if reflect.DeepEqual(canonical(inst), canonical(src)) {
		log.Info("Unmodified from source.")
		overridden = false
		// If not modified, unset object conditions if they exist.
		unsetObj := ns.UnsetCondition(sak, api.ObjectOverridden, msgObj)
		unsetDsc := sns.UnsetCondition(ak, api.ObjectDescendantOverridden, msgDsc)
		// Early exist if not object conditions are unset, so don't call HierarchyReconciler.
		if !unsetObj && !unsetDsc {
			return overridden
		}
		log.Info("Instance restored, object conditions unsetted", "ObjectOverridden", nnm, "ObjectDescendentOverridden", snnm)
	} else {
		if ns.HasKeyedCondition(sak, api.ObjectOverridden) {
			log.Info("Modified, but condition already existed.")
			return true
		}
		log.Info("Instance modified, setting object conditions", "ObjectOverridden", nnm, "ObjectDescendentOverridden", snnm)
		ns.SetCondition(sak, api.ObjectOverridden, msgObj)
		sns.SetCondition(ak, api.ObjectDescendantOverridden, msgDsc)
	}

	// Enqueue affected namespaces to HierarchyReconciler to update the conditions.
	r.HierarchyReconciler.enqueueAffected(log, msgObj, nnm)
	r.HierarchyReconciler.enqueueAffected(log, msgDsc, snnm)

	return overridden
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

// getSourceNS gets source namespace if it's set in "api.LabelInheritedFrom" label.
// It returns an empty string if this instance is the source
func getSourceNS(inst *unstructured.Unstructured) string {
	labels := inst.GetLabels()
	if labels == nil {
		// this cannot be a copy
		return ""
	}
	inheritedFrom, _ := labels[api.LabelInheritedFrom]
	return inheritedFrom
}

// propagate copies the object to the child namespaces. No need to do this recursively as the
// controller will automatically pick up the changes in the child and call this method again.
//
// TODO: parallelize?
func (r *ObjectReconciler) propagate(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured, dests []string) error {
	// The HNC can blacklist certain objects.
	if r.isExcluded(log, inst) {
		return nil
	}

	// Find the original source. If the instance doesn't have the inheritedFrom label set, it _is_ the
	// original source.
	srcNS := inst.GetNamespace()
	if il := inst.GetLabels(); il != nil {
		if v, ok := il[api.LabelInheritedFrom]; ok {
			srcNS = v
		}
	}

	// Propagate the object to all destination namespaces.
	for _, dst := range dests {
		// Create an in-memory canonical version, and set the new properties.
		propagated := canonical(inst)
		propagated.SetNamespace(dst)
		labels := propagated.GetLabels()
		labels[api.LabelInheritedFrom] = srcNS
		propagated.SetLabels(labels)

		// Push to the apiserver
		log.Info("Propagating", "dst", dst, "origin", labels[api.LabelInheritedFrom])
		err := r.Update(ctx, propagated)
		if err != nil && errors.IsNotFound(err) {
			err = r.Create(ctx, propagated)
		}
		if err != nil {
			log.Error(err, "Couldn't propagate", "object", propagated)
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
		// Try to find the propagated objects, if they exist
		propagatedNnm := types.NamespacedName{Namespace: child, Name: nnm.Name}
		propagated := &unstructured.Unstructured{}
		propagated.SetGroupVersionKind(r.GVK)
		err := r.Get(ctx, propagatedNnm, propagated)

		// Is it already gone?
		if errors.IsNotFound(err) {
			continue
		}
		// Some other error?
		if err != nil {
			log.Error(err, "Couldn't read propagated object that needs to be deleted", "name", propagatedNnm)
			return err
		}

		// TODO: double-check the label - or maybe just call deleteObsolete?

		// Delete the copy
		log.Info("Deleting", "propagated", propagatedNnm)
		if err := r.Delete(ctx, propagated); err != nil {
			log.Error(err, "Coudln't delete", "propagated", propagated)
			return err
		}
	}

	return nil
}

// canonical returns a canonicalized version of the object - that is, one that has the same name,
// spec and non-HNC labels and annotations, but with the status and all other metadata cleared
// (including, notably, the namespace). The resulting object is suitable to be copied into a new
// namespace, or two canonicalized objects are suitable for being compared via reflect.DeepEqual.
//
// As a side effect, the label and annotation maps are always initialized in the returned value.
func canonical(inst *unstructured.Unstructured) *unstructured.Unstructured {
	// Start with a copy and clear the status and metadata
	c := inst.DeepCopy()
	delete(c.Object, "status")
	delete(c.Object, "metadata")

	// Restore the whitelisted metadata. Name:
	c.SetName(inst.GetName())

	// Non-HNC annotations:
	newAnnots := map[string]string{}
	for k, v := range inst.GetAnnotations() {
		if !strings.HasPrefix(k, api.MetaGroup) {
			newAnnots[k] = v
		}
	}
	c.SetAnnotations(newAnnots)

	// Non-HNC labels:
	newLabels := map[string]string{}
	for k, v := range inst.GetLabels() {
		if !strings.HasPrefix(k, api.MetaGroup) {
			newLabels[k] = v
		}
	}
	c.SetLabels(newLabels)

	return c
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

func affectedObjectKey(inst *unstructured.Unstructured) string {
	gvk := inst.GroupVersionKind()
	return gvk.Group + "/" + gvk.Version + "/" + gvk.Kind + "/" + inst.GetNamespace() + "/" + inst.GetName()
}

func replaceKeyNamespace(key, namespace string) string {
	split := strings.Split(key, "/")
	split[3] = namespace
	return strings.Join(split, "/")
}
