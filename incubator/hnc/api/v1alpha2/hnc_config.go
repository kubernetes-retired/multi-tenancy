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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Constants for resources and well-known names.
const (
	HNCConfigSingleton  = "config"
	HNCConfigSingletons = "hncconfigurations"
	RBACGroup           = "rbac.authorization.k8s.io"
	RoleResource        = "roles"
	RoleKind            = "Role"
	RoleBindingResource = "rolebindings"
	RoleBindingKind     = "RoleBinding"
)

// SynchronizationMode describes propagation mode of objects of the same kind.
// The only three modes currently supported are "Propagate", "Ignore", and "Remove".
// See detailed definition below. An unsupported mode will be treated as "ignore".
type SynchronizationMode string

const (
	// Propagate objects from ancestors to descendants and deletes obsolete descendants.
	Propagate SynchronizationMode = "Propagate"

	// Ignore the modification of this resource. New or changed objects will not be propagated, and
	// obsolete objects will not be deleted. The inheritedFrom label is not removed.  Any unknown mode
	// is treated as Ignore.
	Ignore SynchronizationMode = "Ignore"

	// Remove all existing propagated copies.
	Remove SynchronizationMode = "Remove"
)

const (
	// Condition types.
	ConditionBadTypeConfiguration = "BadConfiguration"
	ConditionOutOfSync            = "OutOfSync"
	// NamespaceCondition is set if there are namespace conditions, which are set
	// in the HierarchyConfiguration objects. The condition reasons would be the
	// condition types in HierarchyConfiguration, e.g. "ActivitiesHalted".
	ConditionNamespace = "NamespaceCondition"

	// Condition reasons for BadConfiguration
	ReasonMultipleConfigsForType = "MultipleConfigurationsForType"
	ReasonResourceNotFound       = "ResourceNotFound"

	// Condition reason for OutOfSync, e.g. errors when creating a reconciler.
	ReasonUnknown = "Unknown"
)

// EnforcedTypes are the types enforced by HNC that they should not show up in
// the spec and only in the status. Any configurations of the enforced types in
// the spec would cause 'MultipleConfigurationsForType' condition.
var EnforcedTypes = []ResourceSpec{
	{Group: RBACGroup, Resource: RoleResource, Mode: Propagate},
	{Group: RBACGroup, Resource: RoleBindingResource, Mode: Propagate},
}

// IsEnforcedType returns true if configuration is on an enforced type.
func IsEnforcedType(grm ResourceSpec) bool {
	for _, tp := range EnforcedTypes {
		if tp.Group == grm.Group && tp.Resource == grm.Resource {
			return true
		}
	}
	return false
}

// ResourceSpec defines the desired synchronization state of a specific resource.
type ResourceSpec struct {
	// Group of the resource defined below. This is used to unambiguously identify
	// the resource. It may be omitted for core resources (e.g. "secrets").
	Group string `json:"group,omitempty"`
	// Resource to be configured.
	Resource string `json:"resource"`
	// Synchronization mode of the kind. If the field is empty, it will be treated
	// as "Propagate".
	// +optional
	// +kubebuilder:validation:Enum=Propagate;Ignore;Remove
	Mode SynchronizationMode `json:"mode,omitempty"`
}

// ResourceStatus defines the actual synchronization state of a specific resource.
type ResourceStatus struct {
	// The API group of the resource being synchronized.
	Group string `json:"group"`

	// The API version used by HNC when propagating this resource.
	Version string `json:"version"`

	// The resource being synchronized.
	Resource string `json:"resource"`

	// Mode describes the synchronization mode of the kind. Typically, it will be the same as the mode
	// in the spec, except when the reconciler has fallen behind or for resources with an enforced
	// default synchronization mode, such as RBAC objects.
	Mode SynchronizationMode `json:"mode,omitempty"`

	// Tracks the number of objects that are being propagated to descendant namespaces. The propagated
	// objects are created by HNC.
	// +kubebuilder:validation:Minimum=0
	// +optional
	NumPropagatedObjects *int `json:"numPropagatedObjects,omitempty"`

	// Tracks the number of objects that are created by users.
	// +kubebuilder:validation:Minimum=0
	// +optional
	NumSourceObjects *int `json:"numSourceObjects,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hncconfigurations,scope=Cluster
// +kubebuilder:storageversion

// HNCConfiguration is a cluster-wide configuration for HNC as a whole. See details in http://bit.ly/hnc-type-configuration
type HNCConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HNCConfigurationSpec   `json:"spec,omitempty"`
	Status HNCConfigurationStatus `json:"status,omitempty"`
}

// HNCConfigurationSpec defines the desired state of HNC configuration.
type HNCConfigurationSpec struct {
	// Resources defines the cluster-wide settings for resource synchronization.
	// Note that 'roles' and 'rolebindings' are pre-configured by HNC with
	// 'Propagate' mode and are omitted in the spec. Any configuration of 'roles'
	// or 'rolebindings' are not allowed. To learn more, see
	// https://github.com/kubernetes-sigs/multi-tenancy/blob/master/incubator/hnc/docs/user-guide/how-to.md#admin-types
	Resources []ResourceSpec `json:"resources,omitempty"`
}

// HNCConfigurationStatus defines the observed state of HNC configuration.
type HNCConfigurationStatus struct {
	// Resources indicates the observed synchronization states of the resources.
	Resources []ResourceStatus `json:"resources,omitempty"`

	// Conditions describes the errors, if any. If there are any conditions with
	// "ActivitiesHalted" reason, this means that HNC cannot function in the
	// affected namespaces. The HierarchyConfiguration object in each of the
	// affected namespaces will have more information. To learn more about
	// conditions, see https://github.com/kubernetes-sigs/multi-tenancy/blob/master/incubator/hnc/docs/user-guide/concepts.md#admin-conditions.
	Conditions []Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// HNCConfigurationList contains a list of HNCConfiguration.
type HNCConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HNCConfiguration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HNCConfiguration{}, &HNCConfigurationList{})
}
