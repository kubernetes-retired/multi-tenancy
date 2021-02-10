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

package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	_ "k8s.io/component-base/metrics/prometheus/workqueue"
)

const (
	SchedulerSubsystem      = "scheduler"
	SuperClusterHealthKey   = "super_cluster_health"
	VirtualClusterHealthKey = "virtual_cluster_health"
)

var (
	SuperClusterHealthStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: SchedulerSubsystem,
			Name:      SuperClusterHealthKey,
			Help:      "Last health scan status for super clusters.",
		},
		[]string{"status"},
	)
	VirtualClusterHealthStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Subsystem: SchedulerSubsystem,
			Name:      VirtualClusterHealthKey,
			Help:      "Last health scan status for virtual clusters.",
		},
		[]string{"status"},
	)
)

var registerMetrics sync.Once

// Register all metrics.
func Register() {
	registerMetrics.Do(func() {
		prometheus.MustRegister(SuperClusterHealthStats)
		prometheus.MustRegister(VirtualClusterHealthStats)
	})
}
