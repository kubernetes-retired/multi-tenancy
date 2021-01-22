/*
Copyright 2021 The Kubernetes Authors.

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
	"time"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/version"
)

const (
	// LabelScheduledCluster is the super cluster the pod schedules to.
	LabelScheduledCluster = "scheduler.tenancy.x-k8s.io/superCluster"

	// LabelScheduledPlacements is the scheduled placements the namespace schedules to.
	LabelScheduledPlacements = "scheduler.tenancy.x-k8s.io/placements"
)

const (
	// Override the client-go default 5 qps and 10 burst, which are too small for mccontroller .
	DefaultSyncerClientQPS   = 1000
	DefaultSyncerClientBurst = 2000

	// DefaultRequestTimeout is set for all client-go request. This is the absolute
	// timeout of the HTTP request, including reading the response body.
	DefaultRequestTimeout = 30 * time.Second

	// If reconcile request keeps failing, stop retrying after MaxReconcileRetryAttempts.
	// According to controller workqueue default rate limiter algorithm, retry 16 times takes around 180 seconds.
	MaxReconcileRetryAttempts = 16

	// StatusCode represents the status of every syncer operations.
	// TODO: more detailed error code
	StatusCodeOK                     = "OK"
	StatusCodeExceedMaxRetryAttempts = "ExceedMaxRetryAttempts"
	StatusCodeError                  = "Error"
	StatusCodeBadRequest             = "BadRequest"
)

// SuperClusterID is initialized when syncer started, it won't change during syncer life cycle.
var SuperClusterID string

// ResourceSyncerUserAgent is the userAgent name when starting resource syncer.
// TODO: make this configurable in Cluster instance.
var ResourceSyncerUserAgent = "resource-syncer/" + version.BriefVersion()
