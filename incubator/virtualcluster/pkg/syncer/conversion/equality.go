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
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// CheckPodEquality check whether super master object and virtual object
// is logical equal.
// notes: we only care about the metadata and pod spec update.
func CheckPodEquality(pPod, vPod *v1.Pod) *v1.Pod {
	var updatedPod *v1.Pod
	updatedMeta := CheckObjectMetaEquality(&pPod.ObjectMeta, &vPod.ObjectMeta)
	if updatedMeta != nil {
		if updatedPod == nil {
			updatedPod = pPod.DeepCopy()
		}
		updatedPod.ObjectMeta = *updatedMeta
	}

	updatedPodSpec := CheckPodSpecEquality(&pPod.Spec, &vPod.Spec)
	if updatedPodSpec != nil {
		if updatedPod == nil {
			updatedPod = pPod.DeepCopy()
		}
		updatedPod.Spec = *updatedPodSpec
	}

	return updatedPod
}

// CheckObjectMetaEquality check whether super master object and virtual object
// is logical equal.
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
func CheckObjectMetaEquality(pObj, vObj *metav1.ObjectMeta) *metav1.ObjectMeta {
	var updatedObj *metav1.ObjectMeta
	if pObj.GenerateName != vObj.GenerateName {
		if updatedObj == nil {
			updatedObj = pObj.DeepCopy()
		}
		updatedObj.GenerateName = vObj.GenerateName
	}

	labels := CheckKVEquality(pObj.Labels, vObj.Labels)
	if len(labels) != 0 {
		if updatedObj == nil {
			updatedObj = pObj.DeepCopy()
		}
		updatedObj.Labels = vObj.Labels
	}

	annotations := CheckKVEquality(pObj.Annotations, vObj.Annotations)
	if len(annotations) != 0 {
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

// CheckKVEquality check the whether super master object and virtual object
// is logical equal. return equal or not. if not, return the updated value.
func CheckKVEquality(pKV, vKV map[string]string) map[string]string {
	// key in virtual more or diff then super
	moreOrDiff := make(map[string]string)

	for vk, vv := range vKV {
		if strings.HasPrefix(vk, "tenancy.x-k8s.io") {
			// tenant pod should not use this key. it may conflicts with syncer.
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
		if strings.HasPrefix(pk, "tenancy.x-k8s.io") {
			continue
		}

		vv, ok := vKV[pk]
		if !ok {
			less[pk] = vv
		}
	}

	if len(moreOrDiff) == 0 && len(less) == 0 {
		return nil
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

	return updated
}

// CheckPodSpecEquality check the whether super master object and virtual object
// is logical equal. If so, return the updated super master object, else nil.
// Mutable fields:
// - spec.containers[*].image
// - spec.initContainers[*].image
// - spec.activeDeadlineSeconds
func CheckPodSpecEquality(pObj, vObj *v1.PodSpec) *v1.PodSpec {
	var updatedPodSpec *v1.PodSpec

	equal, val := CheckInt64Equality(pObj.ActiveDeadlineSeconds, vObj.ActiveDeadlineSeconds)
	if !equal {
		if updatedPodSpec == nil {
			updatedPodSpec = pObj.DeepCopy()
		}
		updatedPodSpec.ActiveDeadlineSeconds = val
	}

	updatedContainer := CheckContainersImageEquality(pObj.Containers, vObj.Containers)
	if len(updatedContainer) != 0 {
		if updatedPodSpec == nil {
			updatedPodSpec = pObj.DeepCopy()
		}
		updatedPodSpec.Containers = updatedContainer
	}

	updatedContainer = CheckContainersImageEquality(pObj.InitContainers, vObj.InitContainers)
	if len(updatedContainer) != 0 {
		if updatedPodSpec == nil {
			updatedPodSpec = pObj.DeepCopy()
		}
		updatedPodSpec.InitContainers = updatedContainer
	}

	return updatedPodSpec
}

// CheckContainersImageEquality check name:image key-value is equal.
func CheckContainersImageEquality(pObj, vObj []v1.Container) []v1.Container {
	pNameImageMap := make(map[string]string)
	for _, v := range pObj {
		pNameImageMap[v.Name] = v.Image
	}
	vNameImageMap := make(map[string]string)
	for _, v := range vObj {
		vNameImageMap[v.Name] = v.Image
	}

	diff := CheckKVEquality(pNameImageMap, vNameImageMap)
	if len(diff) == 0 {
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

// CheckInt64Equality check the whether super master object and virtual object
// is logical equal. return equal or not. if not, return the updated value.
func CheckInt64Equality(pObj, vObj *int64) (bool, *int64) {
	if pObj == nil && vObj == nil {
		return true, nil
	}

	if pObj != nil && vObj != nil {
		return *pObj == *vObj, pointer.Int64Ptr(*vObj)
	}

	var updated *int64
	if vObj != nil {
		updated = pointer.Int64Ptr(*vObj)
	}

	return false, updated
}

func CheckConfigMapEquality(pObj, vObj *v1.ConfigMap) *v1.ConfigMap {
	var updated *v1.ConfigMap
	updatedMeta := CheckObjectMetaEquality(&pObj.ObjectMeta, &vObj.ObjectMeta)
	if updatedMeta != nil {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.ObjectMeta = *updatedMeta
	}

	updatedData := CheckMapEquality(pObj.Data, vObj.Data)
	if len(updatedData) != 0 {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.Data = updatedData
	}

	updateBinaryData := CheckBinaryDataEquality(pObj.BinaryData, vObj.BinaryData)
	if len(updateBinaryData) != 0 {
		if updated == nil {
			updated = pObj.DeepCopy()
		}
		updated.BinaryData = updateBinaryData
	}

	return updated
}

func CheckMapEquality(pObj, vObj map[string]string) map[string]string {
	if equality.Semantic.DeepEqual(pObj, vObj) {
		return nil
	}

	// deep copy
	updated := make(map[string]string, len(vObj))
	for k, v := range vObj {
		updated[k] = v
	}

	return updated
}

func CheckBinaryDataEquality(pObj, vObj map[string][]byte) map[string][]byte {
	if equality.Semantic.DeepEqual(pObj, vObj) {
		return nil
	}

	// deep copy
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

	return updated
}
