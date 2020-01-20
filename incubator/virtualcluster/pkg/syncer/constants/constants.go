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

package constants

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// LabelCluster records which cluster this resource belongs to.
	LabelCluster = "tenancy.x-k8s.io/cluster"
	// LabelUID is the uid in the tenant namespace.
	LabelUID = "tenancy.x-k8s.io/uid"
	// LabelNamespace records which cluster namespace this resource belongs to.
	LabelNamespace = "tenancy.x-k8s.io/namespace"
	// LabelOwnerReferences is the ownerReferences of the object in tenant context.
	LabelOwnerReferences = "tenancy.x-k8s.io/ownerReferences"

	// SyncStatusKey is a label key records the sync status of the resource.
	SyncStatusKey = "tenancy.x-k8s.io/sync.status"
	// SyncStatusNotReady means the resource has not synced.
	SyncStatusNotReady = "NotReady"
	// SyncStatusReady means the resource has synced.
	SyncStatusReady = "Ready"

	// DefaultControllerWorkers is the quantity of the worker routine for a controller.
	DefaultControllerWorkers = 3

	// ResourceSyncerUserAgent is the userAgent name when starting resource syncer.
	ResourceSyncerUserAgent = "resource-syncer"

	TenantDNSServerNS          = "kube-system"
	TenantDNSServerServiceName = "kube-dns"

	// PublicObjectKey is a label key which marks the super master object that should be populated to every tenant master.
	PublicObjectKey = "tenancy.x-k8s.io/super.public"
)

const (
	// TODO(zhuangqh): make extend info plugable
	// LabelExtendDeploymentName is the parent deployment name of pod. only take effect on pods.
	LabelExtendDeploymentName = "tenancy.x-k8s.io/extend.deployment.name"
	// LabelExtendDeploymentUID is the parent deployment uid of pod. only take effect on pods.
	LabelExtendDeploymentUID = "tenancy.x-k8s.io/extend.deployment.uid"
)

var DefaultDeletionPolicy = metav1.DeletePropagationBackground
