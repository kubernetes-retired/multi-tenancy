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
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	v1a2 "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

var (
	// Map of modes from v1alpha1 to v1alpha2
	toV1A2 = map[SynchronizationMode]v1a2.SynchronizationMode{
		Propagate: v1a2.Propagate,
		Ignore:    v1a2.Ignore,
		Remove:    v1a2.Remove,
	}
)

// ConvertTo converts from this version v1alpha1 to the Hub version v1alpha2.
func (src *HNCConfiguration) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*v1a2.HNCConfiguration)

	// Spec
	srcSpecTypes := src.Spec.Types
	dstSpecRscs := []v1a2.ResourceSpec{}
	for _, st := range srcSpecTypes {
		dr := v1a2.ResourceSpec{}
		// Hack the group from APIVersion by removing the version, e.g.
		// 1) "rbac.authorization.k8s.io/v1" => "rbac.authorization.k8s.io";
		// 2) "v1" => "" (for core type).
		gv := strings.Split(st.APIVersion, "/")
		if len(gv) == 2 {
			dr.Group = gv[0]
		}
		// Hack the resource from Kind by using the lower case and plural form, e.g.
		// 1) "Role" => "roles"
		// 2) "NetworkPolicy" => "networkpolicies"
		lk := strings.ToLower(st.Kind)
		if strings.HasSuffix(lk, "y") {
			lk = strings.TrimSuffix(lk, "y") + "ie"
		}
		dr.Resource = lk + "s"
		dtm, ok := toV1A2[st.Mode]
		if !ok {
			// This should never happen with the enum schema validation.
			dtm = v1a2.Ignore
		}
		dr.Mode = dtm
		dstSpecRscs = append(dstSpecRscs, dr)
	}
	dst.Spec.Resources = dstSpecRscs

	// We don't need to convert status because controllers will update it.
	dst.Status = v1a2.HNCConfigurationStatus{}

	// rote conversion - ObjectMeta
	dst.ObjectMeta = src.ObjectMeta

	return nil
}

// We don't need a conversion from v1alpha2 to v1alpha1. because we will never serve both versions.
// However, the apiserver appears to ask for v1alpha1 even if there are no other clients and
// complains if the metadata changes, so simply copy the metadata but nothing else.
func (dst *HNCConfiguration) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*v1a2.HNCConfiguration)
	dst.ObjectMeta = src.ObjectMeta
	return nil
}
