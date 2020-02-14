/*
Copyright 2019 The Kubernetes Authors.

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

package conversion

import (
	"strings"

	v1 "k8s.io/api/core/v1"
	v1storage "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
)

type vcEquality struct {
	vcSpec *v1alpha1.VirtualclusterSpec
}

func Equality(spec *v1alpha1.VirtualclusterSpec) *vcEquality {
	return &vcEquality{vcSpec: spec}
}

// CheckPodEquality check whether super master Pod object and virtual Pod object
// are logically equal. The source of truth is virtual Pod.
// notes: we only care about the metadata and pod spec update.
func (e vcEquality) CheckPodEquality(pPod, vPod *v1.Pod) *v1.Pod {
	var updatedPod *v1.Pod
	updatedMeta := e.checkDWObjectMetaEquality(&pPod.ObjectMeta, &vPod.ObjectMeta)
	if updatedMeta != nil {
		if updatedPod == nil {
			updatedPod = pPod.DeepCopy()
		}
		updatedPod.ObjectMeta = *updatedMeta
	}

	updatedPodSpec := e.checkPodSpecEquality(&pPod.Spec, &vPod.Spec)
	if updatedPodSpec != nil {
		if updatedPod == nil {
			updatedPod = pPod.DeepCopy()
		}
		updatedPod.Spec = *updatedPodSpec
	}

	return updatedPod
}

// checkDWObjectMetaEquality check whether super master object meta and virtual object meta
// are logically equal. The source of truth is virtual object.
// Reference to ObjectMetaUpdateValidation: https://github.com/kubernetes/kubernetes/blob/release-1.15/staging/src/k8s.io/apimachinery/pkg/api/validation/objectmeta.go#L227
// Mutable fields:
// - generateName
// - labels
// - annotations
// - ownerReferences: ignore. ownerReferences is observed by tenant controller.
// - initializers: ignore. deprecated field and will be removed in v1.15.
// - finalizers: ignore. finalizer is observed by tenant controller.
// - clusterName
// - managedFields: ignore. observed by tenant. https://kubernetes.io/docs/reference/using-api/api-concepts/#field-management
func (e vcEquality) checkDWObjectMetaEquality(pObj, vObj *metav1.ObjectMeta) *metav1.ObjectMeta {
	var updatedObj *metav1.ObjectMeta
	if pObj.GenerateName != vObj.GenerateName {
		if updatedObj == nil {
			updatedObj = pObj.DeepCopy()
		}
		updatedObj.GenerateName = vObj.GenerateName
	}

	labels, equal := e.checkDWKVEquality(pObj.Labels, vObj.Labels)
	if !equal {
		if updatedObj == nil {
			updatedObj = pObj.DeepCopy()
		}
		updatedObj.Labels = labels
	}

	annotations, equal := e.checkDWKVEquality(pObj.Annotations, vObj.Annotations)
	if !equal {
		if updatedObj == nil {
			updatedObj = pObj.DeepCopy()
		}
		updatedObj.Annotations = annotations
	}

	if pObj.ClusterName != vObj.ClusterName {
		if updatedObj == nil {
			updatedObj = pObj.DeepCopy()
		}
		updatedObj.ClusterName = vObj.ClusterName
	}

	return updatedObj
}

func hasPrefixInArray(key string, array []string) bool {
	for _, item := range array {
		if strings.HasPrefix(key, item) {
			return true
		}
	}
	return false
}

// CheckUWObjectMetaEquality mainly checks if super master label or annotations defined in
// VC.Spec.TransparentMetaPrefixes are back populated to tenant master.
func (e vcEquality) CheckUWObjectMetaEquality(pObj, vObj *metav1.ObjectMeta) *metav1.ObjectMeta {
	var updatedObj *metav1.ObjectMeta
	labels, equal := e.checkUWKVEquality(pObj.Labels, vObj.Labels)
	if !equal {
		if updatedObj == nil {
			updatedObj = vObj.DeepCopy()
		}
		updatedObj.Labels = labels
	}

	annotations, equal := e.checkUWKVEquality(pObj.Annotations, vObj.Annotations)
	if !equal {
		if updatedObj == nil {
			updatedObj = vObj.DeepCopy()
		}
		updatedObj.Annotations = annotations
	}
	return updatedObj
}

// checkUWKVEquality checks if any key in VC.Spec.TransparentMetaPrefixes that exists in pKV
// does exist in vKV with the same value.
// Note that we cannot remove a key from tenant if the key was presented in VC.Spec.TransparentMetaPrefixes
// since we did not track the key removal event.
func (e vcEquality) checkUWKVEquality(pKV, vKV map[string]string) (map[string]string, bool) {
	if e.vcSpec == nil {
		return nil, true
	}
	moreOrDiff := make(map[string]string)
	for pk, pv := range pKV {
		if hasPrefixInArray(pk, e.vcSpec.TransparentMetaPrefixes) {
			vv, ok := vKV[pk]
			if !ok || pv != vv {
				moreOrDiff[pk] = pv
			}
		}
	}
	if len(moreOrDiff) == 0 {
		return nil, true
	}
	updated := make(map[string]string)
	for k, v := range vKV {
		updated[k] = v
	}
	for k, v := range moreOrDiff {
		updated[k] = v
	}
	return updated, false
}

// checkDWKVEquality check the whether super master object labels and virtual object labels
// are logically equal. If not, return the updated value. The source of truth is virtual object.
// The exceptional keys that used by super master object are specified in
// VC.Spec.TransparentMetaPrefixes plus a white list (e.g., tenancy.x-k8s.io).
func (e vcEquality) checkDWKVEquality(pKV, vKV map[string]string) (map[string]string, bool) {
	exceptions := []string{}
	if e.vcSpec != nil {
		exceptions = e.vcSpec.TransparentMetaPrefixes
		exceptions = append(exceptions, e.vcSpec.OpaqueMetaPrefixes...)
	}

	// key in virtual more or diff then super
	moreOrDiff := make(map[string]string)

	for vk, vv := range vKV {
		if hasPrefixInArray(vk, exceptions) {
			// tenant pod should not use exceptional keys. it may conflicts with syncer.
			continue
		}
		pv, ok := pKV[vk]
		if !ok || pv != vv {
			moreOrDiff[vk] = vv
		}
	}

	// key in virtual less then super
	less := make(map[string]string)
	for pk := range pKV {
		if hasPrefixInArray(pk, exceptions) {
			continue
		}

		vv, ok := vKV[pk]
		if !ok {
			less[pk] = vv
		}
	}

	if len(moreOrDiff) == 0 && len(less) == 0 {
		return nil, true
	}

	updated := make(map[string]string)
	for k, v := range pKV {
		if _, ok := less[k]; ok {
			continue
		}
		updated[k] = v
	}
	for k, v := range moreOrDiff {
		updated[k] = v
	}

	return updated, false
}

// checkPodSpecEquality check the whether super master Pod Spec and virtual object
// PodSpec are logically equal. The source of truth is virtual Pod Spec.
// Mutable fields:
// - spec.containers[*].image
// - spec.initContainers[*].image
// - spec.activeDeadlineSeconds
func (e vcEquality) checkPodSpecEquality(pObj, vObj *v1.PodSpec) *v1.PodSpec {
	var updatedPodSpec *v1.PodSpec

	val, equal := e.checkInt64Equality(pObj.ActiveDeadlineSeconds, vObj.ActiveDeadlineSeconds)
	if !equal {
		if updatedPodSpec == nil {
			updatedPodSpec = pObj.DeepCopy()
		}
		updatedPodSpec.ActiveDeadlineSeconds = val
	}

	updatedContainer := e.checkContainersImageEquality(pObj.Containers, vObj.Containers)
	if len(updatedContainer) != 0 {
		if updatedPodSpec == nil {
			updatedPodSpec = pObj.DeepCopy()
		}
		updatedPodSpec.Containers = updatedContainer
	}

	updatedContainer = e.checkContainersImageEquality(pObj.InitContainers, vObj.InitContainers)
	if len(updatedContainer) != 0 {
		if updatedPodSpec == nil {
			updatedPodSpec = pObj.DeepCopy()
		}
		updatedPodSpec.InitContainers = updatedContainer
	}

	return updatedPodSpec
}

func (e vcEquality) checkContainersImageEquality(pObj, vObj []v1.Container) []v1.Container {
	pNameImageMap := make(map[string]string)
	for _, v := range pObj {
		pNameImageMap[v.Name] = v.Image
	}
	vNameImageMap := make(map[string]string)
	for _, v := range vObj {
		vNameImageMap[v.Name] = v.Image
	}

	diff, equal := e.checkDWKVEquality(pNameImageMap, vNameImageMap)
	if equal {
		return nil
	}

	for i, v := range pObj {
		if diff[v.Name] == v.Image {
			continue
		}
		c := v.DeepCopy()
		c.Image = diff[v.Name]
		pObj[i] = *c
	}

	return pObj
}

func (e vcEquality) checkInt64Equality(pObj, vObj *int64) (*int64, bool) {
	if pObj == nil && vObj == nil {
		return nil, true
	}

	if pObj != nil && vObj != nil {
		return pointer.Int64Ptr(*vObj), *pObj == *vObj
	}

	var updated *int64
	if vObj != nil {
		updated = pointer.Int64Ptr(*vObj)
	}

	return updated, false
}

// CheckConfigMapEqualit checks whether super master ConfigMap and virtual ConfigMap
// are logically equal. The source of truth is virtual object.
func (e vcEquality) CheckConfigMapEquality(pObj, vObj *v1.ConfigMap) *v1.ConfigMap {
	var updated *v1.ConfigMap
	updatedMeta := e.checkDWObjectMetaEquality(&pObj.ObjectMeta, &vObj.ObjectMeta)
	if updatedMeta != nil {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.ObjectMeta = *updatedMeta
	}

	updatedData, equal := e.checkMapEquality(pObj.Data, vObj.Data)
	if !equal {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.Data = updatedData
	}

	updateBinaryData, equal := e.CheckBinaryDataEquality(pObj.BinaryData, vObj.BinaryData)
	if !equal {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.BinaryData = updateBinaryData
	}

	return updated
}

func (e vcEquality) checkMapEquality(pObj, vObj map[string]string) (map[string]string, bool) {
	if equality.Semantic.DeepEqual(pObj, vObj) {
		return nil, true
	}

	// deep copy
	if vObj == nil {
		return nil, false
	}
	updated := make(map[string]string, len(vObj))
	for k, v := range vObj {
		updated[k] = v
	}

	return updated, false
}

func (e vcEquality) CheckBinaryDataEquality(pObj, vObj map[string][]byte) (map[string][]byte, bool) {
	if equality.Semantic.DeepEqual(pObj, vObj) {
		return nil, true
	}

	// deep copy
	if vObj == nil {
		return nil, false
	}
	updated := make(map[string][]byte, len(vObj))
	for k, v := range vObj {
		if v == nil {
			updated[k] = nil
			continue
		}

		arr := make([]byte, len(v))
		copy(arr, v)
		updated[k] = arr
	}

	return updated, false
}

func (e vcEquality) CheckSecretEquality(pObj, vObj *v1.Secret) *v1.Secret {
	// ignore service account token type secret.
	if vObj.Type == v1.SecretTypeServiceAccountToken {
		return nil
	}

	var updated *v1.Secret
	updatedMeta := e.checkDWObjectMetaEquality(&pObj.ObjectMeta, &vObj.ObjectMeta)
	if updatedMeta != nil {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.ObjectMeta = *updatedMeta
	}

	updatedData, equal := e.checkMapEquality(pObj.StringData, vObj.StringData)
	if !equal {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.StringData = updatedData
	}

	updateBinaryData, equal := e.CheckBinaryDataEquality(pObj.Data, vObj.Data)
	if !equal {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.Data = updateBinaryData
	}

	return updated
}

func (e vcEquality) CheckEndpointsEquality(pObj, vObj *v1.Endpoints) *v1.Endpoints {
	var updated *v1.Endpoints
	updatedMeta := e.checkDWObjectMetaEquality(&pObj.ObjectMeta, &vObj.ObjectMeta)
	if updatedMeta != nil {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.ObjectMeta = *updatedMeta
	}

	if !equality.Semantic.DeepEqual(pObj.Subsets, vObj.Subsets) {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.Subsets = vObj.DeepCopy().Subsets
	}

	return updated
}

func (e vcEquality) CheckStorageClassEquality(pObj, vObj *v1storage.StorageClass) *v1storage.StorageClass {
	pObjCopy := pObj.DeepCopy()
	pObjCopy.ObjectMeta = vObj.ObjectMeta
	// pObj.TypeMeta is empty
	pObjCopy.TypeMeta = vObj.TypeMeta

	if !equality.Semantic.DeepEqual(vObj, pObjCopy) {
		return pObjCopy
	} else {
		return nil
	}
}

func filterNodePort(svc *v1.Service) *v1.ServiceSpec {
	specClone := svc.Spec.DeepCopy()
	for i, _ := range specClone.Ports {
		specClone.Ports[i].NodePort = 0
	}
	return specClone
}

func (e vcEquality) CheckServiceEquality(pObj, vObj *v1.Service) *v1.Service {
	var updated *v1.Service
	updatedMeta := e.checkDWObjectMetaEquality(&pObj.ObjectMeta, &vObj.ObjectMeta)
	if updatedMeta != nil {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.ObjectMeta = *updatedMeta
	}

	// Super/tenant service ClusterIP cannot be the same
	vSpec := filterNodePort(vObj)
	pSpec := filterNodePort(pObj)
	vSpec.ClusterIP = pSpec.ClusterIP

	if !equality.Semantic.DeepEqual(vSpec, pSpec) {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.Spec = *vObj.Spec.DeepCopy()
	}
	return updated
}

func (e vcEquality) CheckPVCEquality(pObj, vObj *v1.PersistentVolumeClaim) *v1.PersistentVolumeClaim {
	var updated *v1.PersistentVolumeClaim
	// PVC meta can be changed
	updatedMeta := e.checkDWObjectMetaEquality(&pObj.ObjectMeta, &vObj.ObjectMeta)
	if updatedMeta != nil {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.ObjectMeta = *updatedMeta
	}
	// ExpandPersistentVolumes feature allows storage size to be increased.
	if pObj.Spec.Resources.Requests["storage"] != vObj.Spec.Resources.Requests["storage"] {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		if updated.Spec.Resources.Requests == nil {
			updated.Spec.Resources.Requests = make(map[v1.ResourceName]resource.Quantity)
		}
		updated.Spec.Resources.Requests["storage"] = vObj.Spec.Resources.Requests["storage"]
	}
	// We don't check PVC status since it will be managed by tenant/master pv binder controller independently.
	return updated
}

func (e vcEquality) CheckPVSpecEquality(pObj, vObj *v1.PersistentVolumeSpec) *v1.PersistentVolumeSpec {
	var updatedPVSpec *v1.PersistentVolumeSpec
	pCopy := pObj.DeepCopy()
	pCopy.ClaimRef = vObj.ClaimRef.DeepCopy()
	if !equality.Semantic.DeepEqual(vObj, pCopy) {
		updatedPVSpec = pCopy
	}
	return updatedPVSpec
}
