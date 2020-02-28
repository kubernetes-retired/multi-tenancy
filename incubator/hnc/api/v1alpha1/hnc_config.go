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
	HNCConfigSingleton = "config"
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
	CritSingletonNameInvalid       HNCConfigurationCode = "critSingletonNameInvalid"
	ObjectReconcilerCreationFailed HNCConfigurationCode = "objectReconcilerCreationFailed"
)

// TypeSynchronizationSpec defines the desired synchronization state of a specific kind.
type TypeSynchronizationSpec struct {
	// API version of the kind defined below. This is used to unambiguously identifies the kind.
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind to be configured.
	Kind string `json:"kind,omitempty"`
	// Synchronization mode of the kind.
	// +optional
	Mode SynchronizationMode `json:"mode,omitempty"`
}

// TypeSynchronizationStatus defines the observed synchronization state of a specific kind.
type TypeSynchronizationStatus struct {
	// API version of the kind defined below. This is used to unambiguously identifies the kind.
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind to be configured.
	Kind string `json:"kind,omitempty"`

	// Tracks the number of original objects that are being propagated to descendant namespaces.
	// +kubebuilder:validation:Minimum=0
	// +optional
	NumPropagated *int32 `json:"numPropagated,omitempty"`
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
	// All codes that begin with the prefix `crit` indicate that reconciliation has
	// been paused for this configuration. Future changes of the configuration will be
	// ignored by HNC until the condition has been resolved. Non-critical conditions
	// typically indicate some kinds of error that HNC itself can ignore. However,
	// the behaviors of some types might be out-of-sync with the users' expectations.
	//
	// Currently, the supported values are:
	//
	// - "critSingletonNameInvalid": the specified singleton name is invalid. The name should be the
	// same as HNCConfigSingleton.
	//
	// - "objectReconcilerCreationFailed": an error exists when creating the object
	// reconciler for the type specified in Msg.
	Code HNCConfigurationCode `json:"code"`

	// A human-readable description of the condition, if the `code` field is not
	// sufficiently clear on their own. If the condition is only for specific types,
	// Msg will include information about the types (e.g., GVK).
	Msg string `json:"msg,omitempty"`
}

func init() {
	SchemeBuilder.Register(&HNCConfiguration{}, &HNCConfigurationList{})
}
