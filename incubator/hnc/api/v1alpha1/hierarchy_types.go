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

// Constants for types and well-known names
const (
	Singleton = "hierarchy"
	Resource  = "hierarchyconfigurations"
)

// Constants for labels and annotations
const (
	MetaGroup          = "hnc.x-k8s.io"
	LabelInheritedFrom = MetaGroup + "/inheritedFrom"
)

// Condition codes. *All* codes must also be documented in the comment to Condition.Code.
const (
	CritParentMissing     Code = "CRIT_PARENT_MISSING"
	CritParentInvalid     Code = "CRIT_PARENT_INVALID"
	CritAncestor          Code = "CRIT_ANCESTOR"
	RequiredChildConflict Code = "REQUIRED_CHILD_CONFLICT"
	CannotUpdate          Code = "CANNOT_UPDATE_OBJECT"
	CannotPropagate       Code = "CANNOT_PROPAGATE_OBJECT"
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

// Code is the machine-readable, enum-like type of `Condition.code`. See that field for more
// information.
type Code string

// Condition specifies the condition and the affected objects.
type Condition struct {
	// Describes the condition in a machine-readable string value. The currently valid values are
	// shown below, but new values may be added over time. This field is always present in a
	// condition.
	//
	// All codes that begin with the prefix `CRIT_` indicate that all HNC activities (e.g. propagating
	// objects, updating labels) have been paused in this namespaces. HNC will resume updating the
	// namespace once the condition has been resolved. Non-critical conditions typically indicate some
	// kind of error that HNC itself can ignore, but likely indicates that the hierarchical structure
	// is out-of-sync with the users' expectations.
	//
	// If the validation webhooks are working properly, there should typically not be any conditions
	// on any namespaces, although some may appear transiently when the HNC controller is restarted.
	// These should quickly resolve themselves (<30s).
	//
	// Currently, the supported values are:
	//
	// - "CRIT_PARENT_MISSING": the specified parent is missing
	//
	// - "CRIT_PARENT_INVALID": the specified parent is invalid (e.g., would cause a cycle)
	//
	// - "CRIT_ANCESTOR": a critical error exists in an ancestor namespace, so this namespace is no
	// longer being updated either.
	//
	// - "REQUIRED_CHILD_CONFLICT": this namespace has a required child, but a namespace of the same
	// name already exists and is not a child of this namespace. Note that the condition is _not_
	// annotated onto the other namespace; it is considered an error _only_ for the would-be parent
	// namespace.
	Code Code `json:"code"`

	// A human-readable description of the condition, if the `code` and `affects` fields are not
	// sufficiently clear on their own.
	Msg string `json:"msg,omitempty"`

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
