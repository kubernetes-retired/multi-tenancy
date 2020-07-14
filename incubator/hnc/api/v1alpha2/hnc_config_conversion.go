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

package v1alpha2

import (
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1a1 "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
)

var (
	// Map of modes from v1alpha1 to v1alpha2
	toV1A2 = map[v1a1.SynchronizationMode]SynchronizationMode{
		v1a1.Propagate: Propagate,
		v1a1.Ignore:    Ignore,
		v1a1.Remove:    Remove,
	}
)

// ConvertFrom converts from the Hub version v1alpha1 to this version v1alpha2.
// We don't need ConvertTo() from v1alpha2 to v1alpha1 because we will never
// serve both versions. We serve v1alpha1 in 0.5 and v1alpha2 in 0.6. Upgrading
// from 0.5 to 0.6 only needs one-way conversion. Downgrading is not supported.
func (dst *HNCConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1a1.HNCConfiguration)

	// Spec
	srcSpecTypes := src.Spec.Types
	dstSpecTypes := []TypeSynchronizationSpec{}
	for _, st := range srcSpecTypes {
		dt := TypeSynchronizationSpec{}
		dt.APIVersion = st.APIVersion
		dt.Kind = st.Kind
		dtm, ok := toV1A2[st.Mode]
		if !ok {
			// This should never happen with the enum schema validation.
			dtm = Ignore
		}
		dt.Mode = dtm
		dstSpecTypes = append(dstSpecTypes, dt)
	}
	dst.Spec.Types = dstSpecTypes

	// We don't need to convert status because controllers will update it.
	dst.Status = HNCConfigurationStatus{}

	// rote conversion - ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	return nil
}
