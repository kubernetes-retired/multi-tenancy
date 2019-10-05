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

var (
	Singleton = "hierarchy"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:object:root=true

// Hierarchy is the Schema for the hierarchies API
type HierarchyConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HierarchyConfigurationSpec   `json:"spec,omitempty"`
	Status HierarchyConfigurationStatus `json:"status,omitempty"`
}

// HierarchySpec defines the desired state of Hierarchy
type HierarchyConfigurationSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Parent indicates the parent of this namespace, if any.
	Parent string `json:"parent,omitempty"`

	// RequiredChildren indicates the required subnamespaces of this namespace. If they do not exist,
	// the HNC will create them, allowing users without privileges to create namespaces to get child
	// namespaces anyway.
	RequiredChildren []string `json:"requiredChildren,omitempty"`
}

// HierarchyStatus defines the observed state of Hierarchy
type HierarchyConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Children indicates the direct children of this namespace, if any.
	Children   []string    `json:"children,omitempty"`
	Conditions []Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// HierarchyList contains a list of Hierarchy
type HierarchyConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HierarchyConfiguration `json:"items"`
}

type Condition struct {
	Msg string `json:"msg"`
}

func init() {
	SchemeBuilder.Register(&HierarchyConfiguration{}, &HierarchyConfigurationList{})
}
