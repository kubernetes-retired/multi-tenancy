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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// VirtualclusterSpec defines the desired state of Virtualcluster
type VirtualclusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ClusterConfig contains information of the tenant cluster, such as
	// apiversion and cluster domain
	// +optional
	ClusterDomain string `json:"clusterDomain"`

	// The name of the desired cluster version, if not set,
	// config of each component need to be provided respectively
	ClusterVersionName string `json:"clusterVersionName"`

	// The valid period of the tenant cluster PKI, if not set
	// the PKI will never expire (i.e. 10 years)
	// +optional
	PKIExpireDays int64 `json:"pkiExpireDays"`

	// the External Users (kubeconfig) need to be generated and stored to Secret
	// +optional
	ExternalUsers []ExternalUser `json:"externalUsers,omitempty"`

	// The Node Selector for deploying Component
	// +optional
	NodeSelector map[string]string
}

type StatefulSetSvcBundle struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Options allow users to specify command line options for
	// this component
	Options []ComponentOption `json:"options,omitempty"`

	StatefulSet *appsv1.StatefulSet `json:"statefulset,omitempty"`

	// Service that exposes the statefulset
	Service *corev1.Service `json:"service,omitempty"`
}

type ComponentOption struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// value of this options
	// e.g. '*,bootstrapsigner,tokencleaner' for the `--controllers` option
	Value *string `json:"defaultValue,omitempty"`

	// Required
	Required bool `json:"required"`

	// The description of this option
	Description *string `json:"description,omitempty"`
}

type ExternalUser struct {
	// Name of the user, also used as the prefix of the secret Name.
	// eg. if Name is admin, the Secret Name will be admin.kubeconfig
	Name string `json:"name"`

	// TLS Client Cert expire days
	ExpireDays int64 `json:"expireDays"`

	// RBAC Username
	Username string `json:"username"`

	// RBAC Groups
	Groups []string `json:"groups"`
}

// VirtualclusterStatus defines the observed state of Virtualcluster
type VirtualclusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
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

	// All components are ready
	ClusterRunning ClusterPhase = "Running"

	// when update cluster spec, phase will be updating
	ClusterUpdating ClusterPhase = "Updating"

	// Cluster can not be inited, or occur the error that Operator
	// can not recover
	ClusterError ClusterPhase = "Error"
)

type ClusterCondition struct {
	// Cluster Condition Status
	// Can be True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`

	// Last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`

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
