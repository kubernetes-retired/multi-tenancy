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
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Constants for types and well-known names
const (
	Singleton               = "hierarchy"
	HierarchyConfigurations = "hierarchyconfigurations"
)

// Constants for labels and annotations
const (
	MetaGroup                  = "hnc.x-k8s.io"
	LabelInheritedFrom         = MetaGroup + "/inheritedFrom"
	FinalizerHasOwnedNamespace = MetaGroup + "/hasOwnedNamespace"
)

// Condition codes. *All* codes must also be documented in the comment to Condition.Code.
const (
	CritParentMissing Code = "CritParentMissing"
	CritParentInvalid Code = "CritParentInvalid"
	CritAncestor      Code = "CritAncestor"
	HNSMissing        Code = "HNSMissing"
	CannotPropagate   Code = "CannotPropagateObject"
	CannotUpdate      Code = "CannotUpdateObject"
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

	// AllowCascadingDelete indicates if the self-serve subnamespaces of this namespace are allowed
	// to cascading delete.
	AllowCascadingDelete bool `json:"allowCascadingDelete,omitempty"`
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
	// All codes that begin with the prefix `Crit` indicate that all HNC activities (e.g. propagating
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
	// - "CritParentMissing": the specified parent is missing
	//
	// - "CritParentInvalid": the specified parent is invalid (e.g., would cause a cycle)
	//
	// - "CritAncestor": a critical error exists in an ancestor namespace, so this namespace is no
	// longer being updated either.
	//
	// - "HNSMissing": this namespace is an owned namespace (created by reconciling an hns instance),
	// but the hns instance is missing in its owner (referenced in its owner annotation) or the owner
	// namespace is missing.
	//
	// - "CannotPropagateObject": this namespace contains an object that couldn't be propagated to
	// one or more of its descendants. The condition's affect objects will include a list of the
	// copies that couldn't be updated.
	//
	// - "CannotUpdateObject": this namespace has an error when updating a propagated object from its
	// source. The condition's affected object will point to the source object.
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

func NewAffectedNamespace(ns string) AffectedObject {
	return AffectedObject{
		Version: "v1",
		Kind:    "Namespace",
		Name:    ns,
	}
}

func NewAffectedObject(gvk schema.GroupVersionKind, ns, nm string) AffectedObject {
	return AffectedObject{
		Group:     gvk.Group,
		Version:   gvk.Version,
		Kind:      gvk.Kind,
		Namespace: ns,
		Name:      nm,
	}
}

// String should only be used for debug purposes
func (a AffectedObject) String() string {
	// No affected object (i.e. affects this namespace?). Note that this will never be returned by the
	// API, but it is used internally to indicate that the API doesn't need to show an affected
	// object.
	if a.Name == "" {
		return "<local>"
	}

	// No namespace -> it *is* a namespace
	if a.Namespace != "" {
		return a.Namespace
	}

	// Generic object (note that Group may be empty for core objects, don't worry about it)
	return fmt.Sprintf("%s/%s/%s/%s/%s", a.Group, a.Version, a.Kind, a.Namespace, a.Name)
}

func SortAffectedObjects(objs []AffectedObject) {
	sort.Slice(objs, func(i, j int) bool {
		if objs[i].Group != objs[j].Group {
			return objs[i].Group < objs[j].Group
		}
		if objs[i].Version != objs[j].Version {
			return objs[i].Version < objs[j].Version
		}
		if objs[i].Version != objs[j].Version {
			return objs[i].Version < objs[j].Version
		}
		if objs[i].Namespace != objs[j].Namespace {
			return objs[i].Namespace < objs[j].Namespace
		}
		return objs[i].Name < objs[j].Name
	})
}

func init() {
	SchemeBuilder.Register(&HierarchyConfiguration{}, &HierarchyConfigurationList{})
}
