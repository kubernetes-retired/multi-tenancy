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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterVersionSpec defines the desired state of ClusterVersion
type ClusterVersionSpec struct {
	// APIserver configuration of the virtual cluster
	APIServer *StatefulSetSvcBundle `json:"apiServer,omitempty"`

	// Controller-manager configuration of the virtual cluster
	ControllerManager *StatefulSetSvcBundle `json:"controllerManager,omitempty"`

	// ETCD configuration of the virtual cluster
	ETCD *StatefulSetSvcBundle `json:"etcd,omitempty"`
}

// StatefulSetSvcBundle contains a StatefulSet and the Service that exposed
// the StatefulSet
type StatefulSetSvcBundle struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// StatefulSet that manages the specified component
	StatefulSet *appsv1.StatefulSet `json:"statefulset,omitempty"`

	// Service that exposes the StatefulSet
	Service *corev1.Service `json:"service,omitempty"`
}

// ClusterVersionStatus defines the observed state of ClusterVersion
type ClusterVersionStatus struct {
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterVersion is the Schema for the clusterversions API
// +k8s:openapi-gen=true
type ClusterVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterVersionSpec   `json:"spec,omitempty"`
	Status ClusterVersionStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +genclient:nonNamespaced

// ClusterVersionList contains a list of ClusterVersion
type ClusterVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterVersion `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterVersion{}, &ClusterVersionList{})
}
