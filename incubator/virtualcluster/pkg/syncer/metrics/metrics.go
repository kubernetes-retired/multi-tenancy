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
)

const (
	ResourceSyncerSubsystem  = "syncer"
	PodOperationsKey         = "pod_operations_total"
	PodOperationsDurationKey = "pod_operations_duration_seconds"
	PodOperationsErrorsKey   = "pod_operations_errors_total"
	CheckerMissMatchKey      = "checker_missmatch_count"
	CheckerRemedyKey         = "checker_remedy_count"
	CheckerScanDurationKey   = "checker_scan_duaration"
	UWSOperationDurationKey  = "uws_operations_duration_seconds"
)

var (
	PodOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      PodOperationsKey,
			Help:      "Cumulative number of pod operations by operation type.",
		},
		[]string{"operation_type"},
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
	PodOperationsErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      PodOperationsErrorsKey,
			Help:      "Cumulative number of pod operation errors by operation type.",
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
		[]string{"checker_target"},
	)
	UWSOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Subsystem: ResourceSyncerSubsystem,
			Name:      UWSOperationDurationKey,
			Help:      "Duration in seconds of resource uws operation time.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"uws_resource"},
	)
)

var registerMetrics sync.Once

// Register all metrics.
func Register() {
	registerMetrics.Do(func() {
		prometheus.MustRegister(PodOperations)
		prometheus.MustRegister(PodOperationsDuration)
		prometheus.MustRegister(PodOperationsErrors)
		prometheus.MustRegister(CheckerMissMatchStats)
		prometheus.MustRegister(CheckerRemedyStats)
		prometheus.MustRegister(CheckerScanDuration)
		prometheus.MustRegister(UWSOperationDuration)
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

func RecordCheckerScanDuration(checkerTarget string, start time.Time) {
	CheckerScanDuration.WithLabelValues(checkerTarget).Observe(SinceInSeconds(start))
}

func RecordUWSOperationDuration(resource string, start time.Time) {
	UWSOperationDuration.WithLabelValues(resource).Observe(SinceInSeconds(start))
}
