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
	MetaGroup                 = "hnc.x-k8s.io"
	LabelInheritedFrom        = MetaGroup + "/inheritedFrom"
	FinalizerHasSubnamespace  = MetaGroup + "/hasSubnamespace"
	LabelTreeDepthSuffix      = ".tree." + MetaGroup + "/depth"
	AnnotationManagedBy       = MetaGroup + "/managed-by"
	AnnotationManagedByV1A1   = MetaGroup + "/managedBy" // TODO: remove after v0.6 branches (#1177)
	AnnotationPropagatePrefix = "propagate." + MetaGroup

	AnnotationSelector     = AnnotationPropagatePrefix + "/select"
	AnnotationTreeSelector = AnnotationPropagatePrefix + "/treeSelect"
	AnnotationNoneSelector = AnnotationPropagatePrefix + "/none"
)

// Condition codes. *All* codes must also be documented in the comment to Condition.Code, be
// inserted into AllCodes, and must have an entry in ClearConditionCriteria, set in init() in this
// file.
//
// Please keep this list in alphabetic order.
const (
	CritAncestor              Code = "CritAncestor"
	CritCycle                 Code = "CritCycle"
	CritDeletingCRD           Code = "CritDeletingCRD"
	CritParentMissing         Code = "CritParentMissing"
	SubnamespaceAnchorMissing Code = "SubnamespaceAnchorMissing"
)

var AllCodes = []Code{
	CritAncestor,
	CritCycle,
	CritDeletingCRD,
	CritParentMissing,
	SubnamespaceAnchorMissing,
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
	//the source namespace.
	EventCannotUpdate string = "CannotUpdateObject"
)

// ClearConditionCriterion describes when a condition should be automatically cleared based on
// forest changes. See individual constants for better documentation.
type ClearConditionCriterion int

const (
	CCCUnknown ClearConditionCriterion = iota

	// CCCManual indicates that the condition should never be cleared automatically, based on the
	// structure of the forest. Instead, the reconciler that sets the condition is responsible for
	// clearing it as well.
	CCCManual

	// CCCAncestor indicates that the condition should always exist in the namespace's ancestors, and
	// should be cleared if this is no longer true.
	CCCAncestor

	// CCCSubtree indicates that the condition should always exist in the namespace's subtree (that
	// is, the namespace itself or any of its descendants), and should be cleared if this is no longer
	// true.
	CCCSubtree
)

// ClearConditionCriteria is initialized in init(). See ClearConditionCriterion for more details.
var ClearConditionCriteria map[Code]ClearConditionCriterion

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
	// These should quickly resolve themselves (<30s). However, validation webhooks are not perfect,
	// especially if multiple users are modifying the same namespace trees quickly, so it's important
	// to monitor for critical conditions and resolve them if they arise. See the user guide for more
	// information.
	//
	// Currently, the supported values are:
	//
	// - "CritParentMissing": the specified parent is missing and the namespace is an orphan.
	//
	// - "CritCycle": the namespace is a member of a cycle. For example, if namespace B says that its
	// parent is namespace A, but namespace A says that its parent is namespace B, then A and B are in
	// a cycle with each other and both of them will have the CritCycle condition.
	//
	// - "CritDeletingCRD": The HierarchyConfiguration CRD is being deleted. No more objects will be
	// propagated into or out of this namespace. It is expected that the HNC controller will be
	// stopped soon after the CRDs are fully deleted.
	//
	// - "CritAncestor": a critical error exists in an ancestor namespace, so this namespace is no
	// longer being updated either.
	//
	// - "SubnamespaceAnchorMissing": this namespace is a subnamespace, but the anchor referenced in
	// its `subnamespaceOf` annotation does not exist in the parent.
	Code Code `json:"code"`

	// A human-readable description of the condition, if the `code` and `affects` fields are not
	// sufficiently clear on their own.
	Msg string `json:"msg,omitempty"`

	// Affects is a list of group-version-kind-namespace-name that uniquely identifies
	// the object(s) affected by the condition.
	Affects []AffectedObject `json:"affects,omitempty"`
}

func (c Condition) String() string {
	affects := fmt.Sprint(c.Affects)
	msg := c.Msg
	if len(msg) > 100 {
		msg = msg[:100] + "..."
	}
	return fmt.Sprintf("%s: %s (affects %s)", c.Code, msg, affects)
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
	if a.Namespace == "" {
		return a.Name
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
	ClearConditionCriteria = map[Code]ClearConditionCriterion{
		// All conditions on namespaces are set/cleared manually by the HCR
		CritAncestor:              CCCManual,
		CritCycle:                 CCCManual,
		CritDeletingCRD:           CCCManual,
		CritParentMissing:         CCCManual,
		SubnamespaceAnchorMissing: CCCManual,
	}
}
