/*
Copyright 2019 The Kubernetes Authors.

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

// TenantNamespaceSpec defines the desired state of TenantNamespace
type TenantNamespaceSpec struct {
	// Name of the tenant namespace. If not specified, TenantNamespace CR
	// name will be used.
	// +optional
	Name string `json:"name,omitempty"`
}

// TenantNamespaceStatus defines the observed state of TenantNamespace
type TenantNamespaceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TenantNamespace is the Schema for the tenantnamespaces API
// +k8s:openapi-gen=true
type TenantNamespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantNamespaceSpec   `json:"spec,omitempty"`
	Status TenantNamespaceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TenantNamespaceList contains a list of TenantNamespace
type TenantNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TenantNamespace `json:"items"`
}

// +kubebuilder:webhook:groups=tenancy,versions=v1alpha1,resources=tenantnamespaces,verbs=create;update
// +kubebuilder:webhook:name=validating-create-update-tenantnamespace.x-k8s.io
// +kubebuilder:webhook:path=/validating-create-update-tenantnamespace
// +kubebuilder:webhook:type=validating,failure-policy=fail

func init() {
	SchemeBuilder.Register(&TenantNamespace{}, &TenantNamespaceList{})
}
