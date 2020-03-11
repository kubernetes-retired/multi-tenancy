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

package reconciler

import (
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type EventType string

const (
	AddEvent    EventType = "Add"
	UpdateEvent EventType = "Update"
	DeleteEvent EventType = "Delete"
)

// Request contains the information needed by a multicluster Reconciler to Reconcile:
// It ONLY contains the meta that can uniquely identify an object without any state information which can lead to parallel reconcile.
type Request struct {
	ClusterName string
	types.NamespacedName
	UID string
}

type Result reconcile.Result

// Reconciler is the interface used by a Controller to reconcile.
type Reconciler interface {
	Reconcile(Request) (Result, error)
}

type UwsRequest struct {
	Key string
	// Optional, in many cases, the cluster name is unknown when uws request is created
	ClusterName string
}
