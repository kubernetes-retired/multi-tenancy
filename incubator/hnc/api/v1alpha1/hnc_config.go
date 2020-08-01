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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Constants for types and well-known names.
const (
	HNCConfigSingleton  = "config"
	HNCConfigSingletons = "hncconfigurations"
)

// SynchronizationMode describes propagation mode of objects of the same kind.
// The only three modes currently supported are "propagate", "ignore", and "remove".
// See detailed definition below. An unsupported mode will be treated as "ignore".
type SynchronizationMode string

const (
	// Propagate objects from ancestors to descendants and deletes obsolete descendants.
	Propagate SynchronizationMode = "propagate"

	// Ignore the modification of this type. New or changed objects will not be propagated,
	// and obsolete objects will not be deleted. The inheritedFrom label is not removed.
	// Any unknown mode is treated as ignore.
	Ignore SynchronizationMode = "ignore"

	// Remove all existing propagated copies.
	Remove SynchronizationMode = "remove"
)

// HNCConfigurationCondition codes. *All* codes must also be documented in the
// comment to HNCConfigurationCondition.Code.
const (
	ObjectReconcilerCreationFailed   HNCConfigurationCode = "ObjectReconcilerCreationFailed"
	MultipleConfigurationsForOneType HNCConfigurationCode = "MultipleConfigurationsForOneType"
)

// TypeSynchronizationSpec defines the desired synchronization state of a specific kind.
type TypeSynchronizationSpec struct {
	// API version of the kind defined below. This is used to unambiguously identifies the kind.
	APIVersion string `json:"apiVersion"`
	// Kind to be configured.
	Kind string `json:"kind"`
	// Synchronization mode of the kind. If the field is empty, it will be treated
	// as "propagate".
	// +optional
	// +kubebuilder:validation:Enum=propagate;ignore;remove
	Mode SynchronizationMode `json:"mode,omitempty"`
}

// TypeSynchronizationStatus defines the observed synchronization state of a specific kind.
type TypeSynchronizationStatus struct {
	// API version of the kind defined below. This is used to unambiguously identifies the kind.
	APIVersion string `json:"apiVersion"`
	// Kind to be configured.
	Kind string `json:"kind"`
	// Mode describes the synchronization mode of the kind. Typically, it will be the same as the mode
	// in the spec, except when the reconciler has fallen behind or when the mode is omitted from the
	// spec and the default is chosen.
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

type CodeAndAffectedNamespaces struct {
	// Code is a namespace condition code
	Code Code `json:"code"`

	// Namespaces is the list of namespaces affected by this code
	Namespaces []string `json:"namespaces"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hncconfigurations,scope=Cluster

// HNCConfiguration is a cluster-wide configuration for HNC as a whole. See details in http://bit.ly/hnc-type-configuration
type HNCConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HNCConfigurationSpec   `json:"spec,omitempty"`
	Status HNCConfigurationStatus `json:"status,omitempty"`
}

// HNCConfigurationSpec defines the desired state of HNC configuration.
type HNCConfigurationSpec struct {
	// Types indicates the desired synchronization states of kinds, if any.
	Types []TypeSynchronizationSpec `json:"types,omitempty"`
}

// HNCConfigurationStatus defines the observed state of HNC configuration.
type HNCConfigurationStatus struct {
	// Types indicates the observed synchronization states of kinds, if any.
	Types []TypeSynchronizationStatus `json:"types,omitempty"`

	// Conditions describes the errors, if any.
	Conditions []HNCConfigurationCondition `json:"conditions,omitempty"`

	// NamespaceConditions is a map of namespace condition codes to the namespaces affected by those
	// codes. If HNC is operating normally, no conditions will be present; if there are any conditions
	// beginning with the "Crit" (critical) prefix, this means that HNC cannot function in the
	// affected namespaces. The HierarchyConfiguration object in each of the affected namespaces will
	// have more information. To learn more about conditions, see
	// https://github.com/kubernetes-sigs/multi-tenancy/blob/master/incubator/hnc/docs/user-guide/concepts.md#admin-conditions.
	NamespaceConditions []CodeAndAffectedNamespaces `json:"namespaceConditions,omitempty"`
}

// +kubebuilder:object:root=true

// HNCConfigurationList contains a list of HNCConfiguration.
type HNCConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HNCConfiguration `json:"items"`
}

// HNCConfigurationCode is the machine-readable, enum-like type of `HNCConfigurationCondition.Code`.
// See that field for more information.
type HNCConfigurationCode string

// HNCConfigurationCondition specifies the code and the description of an error condition.
type HNCConfigurationCondition struct {
	// Describes the condition in a machine-readable string value. The currently valid values are
	// shown below, but new values may be added over time. This field is always present in a
	// condition.
	//
	// Conditions typically indicate some kinds of error that HNC itself can ignore. However,
	// the behaviors of some types might be out-of-sync with the users' expectations.
	//
	// Currently, the supported values are:
	//
	// - "objectReconcilerCreationFailed": an error exists when creating the object
	// reconciler for the type specified in Msg.
	//
	// - "multipleConfigurationsForOneType": Multiple configurations exist for the type specified
	// in the Msg. One type should only have one configuration.
	Code HNCConfigurationCode `json:"code"`

	// A human-readable description of the condition, if the `code` field is not
	// sufficiently clear on their own. If the condition is only for specific types,
	// Msg will include information about the types (e.g., GVK).
	Msg string `json:"msg,omitempty"`
}

func init() {
	SchemeBuilder.Register(&HNCConfiguration{}, &HNCConfigurationList{})
}
