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

package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	_ "k8s.io/kubernetes/pkg/util/workqueue/prometheus"
)

const (
	ResourceSyncerSubsystem  = "syncer"
	PodOperationsKey         = "pod_operations_total"
	PodOperationsDurationKey = "pod_operations_duration_seconds"
	CheckerMissMatchKey      = "checker_missmatch_count"
	CheckerRemedyKey         = "checker_remedy_count"
	CheckerScanDurationKey   = "checker_scan_duaration_seconds"
	DWSOperationCounterKey   = "dws_operations_total"
	DWSOperationDurationKey  = "dws_operations_duration_seconds"
	UWSOperationCounterKey   = "uws_operations_total"
	UWSOperationDurationKey  = "uws_operations_duration_seconds"
	ClusterHealthKey         = "virtual_cluster_health"
)

var (
	PodOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      PodOperationsKey,
			Help:      "Cumulative number of pod operations by operation type.",
		},
		[]string{"operation_type", "code"},
	)
	PodOperationsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      PodOperationsDurationKey,
			Help:      "Duration in seconds of pod operations. Broken down by operation type.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"operation_type"},
	)
	CheckerMissMatchStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      CheckerMissMatchKey,
			Help:      "Last checker scan results for mismatched resources.",
		},
		[]string{"counter_name"},
	)
	CheckerRemedyStats = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      CheckerRemedyKey,
			Help:      "Cumulative number of checker remediation actions.",
		},
		[]string{"counter_name"},
	)
	CheckerScanDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      CheckerScanDurationKey,
			Help:      "Duration in seconds of each resource checker's scan time.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"resource"},
	)
	DWSOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      DWSOperationDurationKey,
			Help:      "Duration in seconds of each resource dws operation time.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"resource", "vc_name"})
	DWSOperationCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      DWSOperationCounterKey,
			Help:      "Cumulative number of downward resource operations.",
		},
		[]string{"resource", "vc_name", "code"})
	UWSOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      UWSOperationDurationKey,
			Help:      "Duration in seconds of resource uws operation time.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"resource"},
	)
	UWSOperationCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      UWSOperationCounterKey,
			Help:      "Cumulative number of upward resource operations.",
		},
		[]string{"resource", "code"})
	ClusterHealthStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      ClusterHealthKey,
			Help:      "Last checker scan status for virtual clusters.",
		},
		[]string{"status"},
	)
)

var registerMetrics sync.Once

// Register all metrics.
func Register() {
	registerMetrics.Do(func() {
		prometheus.MustRegister(PodOperations)
		prometheus.MustRegister(PodOperationsDuration)
		prometheus.MustRegister(CheckerMissMatchStats)
		prometheus.MustRegister(CheckerRemedyStats)
		prometheus.MustRegister(CheckerScanDuration)
		prometheus.MustRegister(DWSOperationCounter)
		prometheus.MustRegister(DWSOperationDuration)
		prometheus.MustRegister(UWSOperationDuration)
		prometheus.MustRegister(ClusterHealthStats)
	})
}

// Gets the time since the specified start in microseconds.
func SinceInMicroseconds(start time.Time) float64 {
	return float64(time.Since(start).Nanoseconds() / time.Microsecond.Nanoseconds())
}

// SinceInSeconds gets the time since the specified start in seconds.
func SinceInSeconds(start time.Time) float64 {
	return time.Since(start).Seconds()
}

func RecordCheckerScanDuration(resource string, start time.Time) {
	CheckerScanDuration.WithLabelValues(resource).Observe(SinceInSeconds(start))
}

func RecordUWSOperationDuration(resource string, start time.Time) {
	UWSOperationDuration.With(prometheus.Labels{"resource": resource}).Observe(SinceInSeconds(start))
}

func RecordUWSOperationStatus(resource, code string) {
	UWSOperationCounter.With(prometheus.Labels{"resource": resource, "code": code}).Inc()
}

func RecordDWSOperationDuration(resource, cluster string, start time.Time) {
	DWSOperationDuration.With(prometheus.Labels{"resource": resource, "vc_name": cluster}).Observe(SinceInSeconds(start))
}

func RecordDWSOperationStatus(resource, cluster, code string) {
	DWSOperationCounter.With(prometheus.Labels{"resource": resource, "vc_name": cluster, "code": code}).Inc()
}
