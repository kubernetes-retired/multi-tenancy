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

package v1alpha1

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1a2 "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

// ConvertTo converts from this version v1alpha1 to the Hub version v1alpha2.
func (src *HierarchyConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1a2.HierarchyConfiguration)

	// Spec
	dst.Spec.AllowCascadingDeletion = src.Spec.AllowCascadingDelete
	dst.Spec.Parent = src.Spec.Parent

	// We don't need to convert status because controllers will update it.
	dst.Status = v1a2.HierarchyConfigurationStatus{}

	// rote conversion - ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	return nil
}

// We don't need a conversion from v1alpha2 to v1alpha1. because we will never serve both versions.
// However, the apiserver appears to ask for v1alpha1 even if there are no other clients and
// complains if the metadata changes, so simply copy the metadata but nothing else.
func (dst *HierarchyConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1a2.HierarchyConfiguration)
	dst.ObjectMeta = src.ObjectMeta
	return nil
}
