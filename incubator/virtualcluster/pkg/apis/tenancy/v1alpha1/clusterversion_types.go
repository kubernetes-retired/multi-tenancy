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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ClusterVersionSpec defines the desired state of ClusterVersion
type ClusterVersionSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	APIServer         *StatefulSetSvcBundle `json:"apiServer,omitempty"`
	ControllerManager *StatefulSetSvcBundle `json:"controllerManager,omitempty"`
	ETCD              *StatefulSetSvcBundle `json:"etcd,omitempty"`
}

// ClusterVersionStatus defines the observed state of ClusterVersion
type ClusterVersionStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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

type ClusterVersionHistory struct {
	ClusterVersionName string         `json:"ClusterVersionName"`
	ClusterVersion     ClusterVersion `json:"clusterVersion"`
}

func init() {
	SchemeBuilder.Register(&ClusterVersion{}, &ClusterVersionList{})
}
