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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualclusterSpec defines the desired state of Virtualcluster
type VirtualclusterSpec struct {
	// ClusterDomain is the domain name of the virtual cluster
	// e.g. a pod dns will be
	// {some-pod}.{some-namespace}.svc.{ClusterDomain}
	// +optional
	ClusterDomain string `json:"clusterDomain,omitempty"`

	// The name of the desired cluster version
	ClusterVersionName string `json:"clusterVersionName"`

	// The valid period of the tenant cluster PKI, if not set
	// the PKI will never expire (i.e. 10 years)
	// +optional
	PKIExpireDays int64 `json:"pkiExpireDays,omitempty"`

	// The Node Selector for deploying Component
	// +optional
	NodeSelector map[string]string
}

// VirtualclusterStatus defines the observed state of Virtualcluster
type VirtualclusterStatus struct {
	// cluster phase of the virtual cluster
	Phase ClusterPhase `json:"phase"`

	// A human readable message indicating details about why the cluster is in
	// this condition.
	// +optional
	Message string `json:"message"`

	// A brief CamelCase message indicating details about why the cluster is in
	// this state.
	// e.g. 'Evicted'
	// +optional
	Reason string `json:"reason"`

	// Cluster Conditions
	Conditions []ClusterCondition `json:"conditions,omitempty"`

	// Version publish history
	ClusterVersionHistory []ClusterVersionHistory `json:"versionHistory,omitempty"`
}

type ClusterPhase string

const (
	// Cluster is processed by Operator, but not all components are ready
	ClusterPending ClusterPhase = "Pending"

	// All components are running
	// NOTE when cluster is in this state, pod can't visit the master of
	// the virtualcluster from inside the cluster by using service account
	ClusterRunning ClusterPhase = "Running"

	// when update cluster spec, phase will be updating
	ClusterUpdating ClusterPhase = "Updating"

	// The cluster is ready
	// NOTE cluster in this state allows in-cluster master visiting, and can
	// pass the conformance test of Kubernetes version 1.15
	ClusterReady ClusterPhase = "Ready"

	// Cluster can not be initiated, or occur the error that Operator
	// can not recover
	ClusterError ClusterPhase = "Error"
)

type ClusterCondition struct {
	// Cluster Condition Status
	// Can be True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`

	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`

	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Virtualcluster is the Schema for the virtualclusters API
// +k8s:openapi-gen=true
type Virtualcluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualclusterSpec   `json:"spec,omitempty"`
	Status VirtualclusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// VirtualclusterList contains a list of Virtualcluster
type VirtualclusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Virtualcluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Virtualcluster{}, &VirtualclusterList{})
}
