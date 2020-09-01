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

// Constants for the subnamespace anchor resource type and namespace annotation.
const (
	Anchors          = "subnamespaceanchors"
	AnchorKind       = "SubnamespaceAnchor"
	AnchorAPIVersion = MetaGroup + "/v1alpha1"
	SubnamespaceOf   = MetaGroup + "/subnamespaceOf"
)

// SubnamespaceAnchorState describes the state of the subnamespace. The state could be
// "missing", "ok", "conflict" or "forbidden". The definitions will be described below.
type SubnamespaceAnchorState string

// Anchor states, which are documented in the comment to SubnamespaceAnchorStatus.State.
const (
	Missing   SubnamespaceAnchorState = "missing"
	Ok        SubnamespaceAnchorState = "ok"
	Conflict  SubnamespaceAnchorState = "conflict"
	Forbidden SubnamespaceAnchorState = "forbidden"
)

// SubnamespaceAnchorStatus defines the observed state of SubnamespaceAnchor.
type SubnamespaceAnchorStatus struct {
	// Describes the state of the subnamespace anchor.
	//
	// Currently, the supported values are:
	//
	// - "missing": the subnamespace has not been created yet. This should be the default state when
	// the anchor is just created.
	//
	// - "ok": the subnamespace exists.
	//
	// - "conflict": a namespace of the same name already exists. The admission controller will
	// attempt to prevent this.
	//
	// - "forbidden": the anchor was created in a namespace that doesn't allow children, such as
	// kube-system or hnc-system. The admission controller will attempt to prevent this.
	State SubnamespaceAnchorState `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=subnamespaceanchors,shortName=subns,scope=Namespaced
// +kubebuilder:unservedversion

// SubnamespaceAnchor is the Schema for the subnamespace API.
// See details at http://bit.ly/hnc-self-serve-ux.
type SubnamespaceAnchor struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status SubnamespaceAnchorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SubnamespaceAnchorList contains a list of SubnamespaceAnchor.
type SubnamespaceAnchorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SubnamespaceAnchor `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SubnamespaceAnchor{}, &SubnamespaceAnchorList{})
}
