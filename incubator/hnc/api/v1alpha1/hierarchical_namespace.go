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

// HNSState describes the state of a hierarchical namespace. The state could be
// "missing", "ok", "conflict" or "forbidden". The definitions will be described below.
type HNSState string

// HNSStates, which are documented in the comment to HierarchicalNamespaceStatus.State.
const (
	Missing   HNSState = "missing"
	Ok        HNSState = "ok"
	Conflict  HNSState = "conflict"
	Forbidden HNSState = "forbidden"
)

// HierarchicalNamespaceStatus defines the observed state of HierarchicalNamespace.
type HierarchicalNamespaceStatus struct {
	// Describes the state of a hierarchical namespace.
	//
	// Currently, the supported values are:
	//
	// - "missing": the child namespace has not been created yet. This should be the default
	// state when the HNS is just created.
	//
	// - "ok": the child namespace exists.
	//
	// - "conflict": a namespace of the same name already exists. The admission controller
	// will attempt to prevent this.
	//
	// - "forbidden": the HNS was created in a namespace that doesn't allow children, such
	// as kube-system or hnc-system. The admission controller will attempt to prevent this.
	State HNSState `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hierarchicalnamespaces,shortName=hns,scope=Namespaced

// HierarchicalNamespace is the Schema for the self-service namespace API.
// See details at http://bit.ly/hnc-self-serve-ux.
type HierarchicalNamespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Status HierarchicalNamespaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// HierarchicalNamespaceList contains a list of HierarchicalNamespace.
type HierarchicalNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HierarchicalNamespace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HierarchicalNamespace{}, &HierarchicalNamespaceList{})
}
