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

// Any changes here need to also be reflected in the kubebuilder:validation:Enum comment, below.
// The meanings of all these constants are defined in the comment to Condition.Code, below.
const (
	CritParentMissing         Code = "CRIT_PARENT_MISSING"
	CritParentInvalid         Code = "CRIT_PARENT_INVALID"
	CritRequiredChildConflict Code = "CRIT_REQUIRED_CHILD_CONFLICT"
	CritAncestor              Code = "CRIT_ANCESTOR"
	ObjectOverridden          Code = "OBJECT_OVERRIDDEN"
	ObjectDescendantOverriden Code = "OBJECT_DESCENDANT_OVERRIDDEN"
	MetaGroup                      = "hnc.x-k8s.io"
	LabelInheritedFrom             = MetaGroup + "/inheritedFrom"
	AnnotationModified             = MetaGroup + "/modified"
)

var (
	Singleton = "hierarchy"
	Resource  = "hierarchyconfigurations"
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
	Children []string `json:"children,omitempty"`

	// Conditions describes the errors and the affected objects, if any.
	Conditions []Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// HierarchyList contains a list of Hierarchy
type HierarchyConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HierarchyConfiguration `json:"items"`
}

// Code is a machine-readable string value summarizing the condition.
// +kubebuilder:validation:Enum=CRIT_PARENT_MISSING;CRIT_PARENT_INVALID;CRIT_REQUIRED_CHILD_CONFLICT;CRIT_ANCESTOR;OBJECT_OVERRIDDEN;OBJECT_DESCENDANT_OVERRIDDEN
type Code string

// Condition specifies the condition and the affected objects.
type Condition struct {
	// Defines the conditions in a machine-readable string value.
	// Valid values are:
	//
	// - "CRIT_PARENT_MISSING": the specified parent is missing
	//
	// - "CRIT_PARENT_INVALID": the specified parent is invalid (ie would cause a cycle)
	//
	// - "CRIT_REQUIRED_CHILD_CONFLICT": there's a conflict (ie in between parent's RequiredChildren spec and child's Parent spec)
	//
	// - "CRIT_ANCESTOR": a critical error exists in an ancestor namespace, so this namespace is no longer being updated
	//
	// - "OBJECT_OVERRIDDEN": an object in this namespace has been overridden from its parent and will no longer be updated
	//
	// - "OBJECT_DESCENDANT_OVERRIDDEN": an object in this namespace is no longer being propagated because a propagated copy has been modified
	Code Code   `json:"code,omitempty"`
	Msg  string `json:"msg,omitempty"`

	// Affects is a list of group-version-kind-namespace-name that uniquely identifies
	// the object(s) affected by the condition.
	Affects []AffectedObject `json:"affects,omitempty"`
}

// AffectedObject defines uniquely identifiable objects.
type AffectedObject struct {
	Group     string `json:"group,omitempty"`
	Version   string `json:"version,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`
}

func init() {
	SchemeBuilder.Register(&HierarchyConfiguration{}, &HierarchyConfigurationList{})
}
