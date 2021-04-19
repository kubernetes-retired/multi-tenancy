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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Constants for types and well-known names
const (
	Singleton               = "hierarchy"
	HierarchyConfigurations = "hierarchyconfigurations"
)

// Constants for labels and annotations
const (
	MetaGroup                 = "hnc.x-k8s.io"
	LabelInheritedFrom        = MetaGroup + "/inherited-from"
	FinalizerHasSubnamespace  = MetaGroup + "/hasSubnamespace"
	LabelTreeDepthSuffix      = ".tree." + MetaGroup + "/depth"
	AnnotationManagedBy       = MetaGroup + "/managed-by"
	AnnotationPropagatePrefix = "propagate." + MetaGroup

	AnnotationSelector     = AnnotationPropagatePrefix + "/select"
	AnnotationTreeSelector = AnnotationPropagatePrefix + "/treeSelect"
	AnnotationNoneSelector = AnnotationPropagatePrefix + "/none"

	// LabelManagedByStandard will eventually replace our own managed-by annotation (we didn't know
	// about this standard label when we invented our own).
	LabelManagedByApps = "app.kubernetes.io/managed-by"

	// LabelExcludedNamespace is the label added by users on the namespaces that
	// should be excluded from our validators, e.g. "kube-system".
	LabelExcludedNamespace = MetaGroup + "/excluded-namespace"
)

const (
	// Condition types.
	ConditionActivitiesHalted string = "ActivitiesHalted"
	ConditionBadConfiguration string = "BadConfiguration"

	// Condition reasons.
	ReasonAncestor      string = "AncestorHaltActivities"
	ReasonDeletingCRD   string = "DeletingCRD"
	ReasonInCycle       string = "InCycle"
	ReasonParentMissing string = "ParentMissing"
	ReasonIllegalParent string = "IllegalParent"
	ReasonAnchorMissing string = "SubnamespaceAnchorMissing"
)

// AllConditions have all the conditions by type and reason. Please keep this
// list in alphabetic order. This is specifically used to clear (set to 0)
// conditions in the metrics.
var AllConditions = map[string][]string{
	ConditionActivitiesHalted: {
		ReasonAncestor,
		ReasonDeletingCRD,
		ReasonInCycle,
		ReasonParentMissing,
		ReasonIllegalParent,
	},
	ConditionBadConfiguration: {
		ReasonAnchorMissing,
	},
}

const (
	// EventCannotPropagate is for events when a namespace contains an object that
	// couldn't be propagated *out* of the namespace, to one or more of its
	// descendants. If the object couldn't be propagated to *any* descendants - for
	// example, because it has a finalizer on it (HNC can't propagate objects with
	// finalizers), the error message will point to the object in this namespace.
	// Otherwise, if it couldn't be propagated to *some* descendant, the error
	// message will point to the descendant.
	EventCannotPropagate string = "CannotPropagateObject"
	// EventCannotUpdate is for events when a namespace has an object that couldn't
	// be propagated *into* this namespace - that is, it couldn't be created in
	// the first place, or it couldn't be updated. The error message will point to
	// the source namespace.
	EventCannotUpdate string = "CannotUpdateObject"
	// EventCannotGetSelector is for events when an object has annotations that cannot be
	// parsed into a valid selector
	EventCannotParseSelector string = "CannotParseSelector"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

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

	// AllowCascadingDeletion indicates if the subnamespaces of this namespace are
	// allowed to cascading delete.
	AllowCascadingDeletion bool `json:"allowCascadingDeletion,omitempty"`
}

// HierarchyStatus defines the observed state of Hierarchy
type HierarchyConfigurationStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Children indicates the direct children of this namespace, if any.
	Children []string `json:"children,omitempty"`

	// Conditions describes the errors, if any.
	Conditions []Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// HierarchyList contains a list of Hierarchy
type HierarchyConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HierarchyConfiguration `json:"items"`
}

// metav1.Condition is introduced in k8s.io/apimachinery v0.20.0-alpha.1 and we
// don't want to take a dependency on it yet, thus we copied the below struct from
// https://github.com/kubernetes/apimachinery/blob/master/pkg/apis/meta/v1/types.go:

// Condition contains details for one aspect of the current state of this API Resource.
// ---
// This struct is intended for direct use as an array at the field path .status.conditions.  For example,
// type FooStatus struct{
//     // Represents the observations of a foo's current state.
//     // Known .status.conditions.type are: "Available", "Progressing", and "Degraded"
//     // +patchMergeKey=type
//     // +patchStrategy=merge
//     // +listType=map
//     // +listMapKey=type
//     Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
//
//     // other fields
// }
type Condition struct {
	// type of condition in CamelCase or in foo.example.com/CamelCase.
	// ---
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	// The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MaxLength=316
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`
	// status of the condition, one of True, False, Unknown.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status metav1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status"`
	// observedGeneration represents the .metadata.generation that the condition was set based upon.
	// For instance, if .metadata.generation is currently 12, but the .status.conditions[x].observedGeneration is 9, the condition is out of date
	// with respect to the current state of the instance.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,3,opt,name=observedGeneration"`
	// lastTransitionTime is the last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	LastTransitionTime metav1.Time `json:"lastTransitionTime" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// reason contains a programmatic identifier indicating the reason for the condition's last transition.
	// Producers of specific condition types may define expected values and meanings for this field,
	// and whether the values are considered a guaranteed API.
	// The value should be a CamelCase string.
	// This field may not be empty.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=1024
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[A-Za-z]([A-Za-z0-9_,:]*[A-Za-z0-9_])?$`
	Reason string `json:"reason" protobuf:"bytes,5,opt,name=reason"`
	// message is a human readable message indicating details about the transition.
	// This may be an empty string.
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=32768
	Message string `json:"message" protobuf:"bytes,6,opt,name=message"`
}

// NewCondition fills some required field with default values for schema
// validation, e.g. Status and LastTransitionTime.
func NewCondition(tp, reason, msg string) Condition {
	return Condition{
		Type:   tp,
		Status: "True",
		// Set time as an obviously wrong value 1970-01-01T00:00:00Z since we
		// overwrite conditions every time.
		LastTransitionTime: metav1.Unix(0, 0),
		Reason:             reason,
		Message:            msg,
	}
}

func (c Condition) String() string {
	msg := c.Message
	if len(msg) > 100 {
		msg = msg[:100] + "..."
	}
	return fmt.Sprintf("%s (%s): %s", c.Type, c.Reason, msg)
}

func init() {
	SchemeBuilder.Register(&HierarchyConfiguration{}, &HierarchyConfigurationList{})
}
