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

// We don't need a conversion from v1alpha2 to v1alpha1. because we will never
// serve both versions. We serve v1alpha1 in 0.5 and v1alpha2 in 0.6. Upgrading
// from 0.5 to 0.6 only needs one-way conversion. Downgrading is not supported.
// Thus we keep an empty ConvertFrom just to implement Convertible.
func (dst *HierarchyConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	// We wanted to return errors.New("not supported") here considering this
	// function should never be called, but in reality this error log is populated
	// constantly even when all the HNC reconcilers and validators are disabled.
	// To not pollute the logs, we decide to return nil here.
	return nil
}
