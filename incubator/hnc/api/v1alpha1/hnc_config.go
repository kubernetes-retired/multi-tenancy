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

// SynchronizationMode describes propogation mode of objects of the same kind.
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
