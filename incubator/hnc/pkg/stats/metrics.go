package stats

import (
	"context"
	"sync"
	"time"

	ocstats "go.opencensus.io/stats"
	ocview "go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
)

// ReportingInterval is the exporter reporting period.
const ReportingInterval = 1 * time.Minute

// Create Measures. A measure represents a metric type to be recorded.
var (
	hierConfigReconcileTotal      = ocstats.Int64("hierconfig_reconcile_total", "The total number of HierConfig reconciliations happened", "reconciliations")
	hierConfigReconcileConcurrent = ocstats.Int64("hierconfig_reconcile_concurrent_peak", "The peak concurrent HierConfig reconciliations happened in the last reporting period", "reconciliations")
	hierConfigWritesTotal         = ocstats.Int64("hierconfig_writes_total", "The number of HierConfig writes happened during HierConfig reconciliations", "writes")
	namespaceWritesTotal          = ocstats.Int64("namespace_writes_total", "The number of namespace writes happened during HierConfig reconciliations", "writes")
	objectReconcileTotal          = ocstats.Int64("object_reconcile_total", "The total number of object reconciliations happened", "reconciliations")
	objectReconcileConcurrent     = ocstats.Int64("object_reconcile_concurrent_peak", "The peak concurrent object reconciliations happened in the last reporting period", "reconciliations")
	objectWritesTotal             = ocstats.Int64("object_writes_total", "The number of object writes happened during object reconciliations", "writes")
)

// Create Tags. Tags are used to group and filter collected metrics later on.
// Create a GroupKind Tag for metrics of object reconcilers for different GroupKind.
var KeyGroupKind, _ = tag.NewKey("GroupKind")

// Create Views. Views are the coupling of an Aggregation applied to a Measure and
// optionally Tags. Views are the connection to Metric exporters.
var (
	hierReconcileTotalView = &ocview.View{
		Name:        "hnc/reconcilers/hierconfig/total",
		Measure:     hierConfigReconcileTotal,
		Description: "The total number of HierConfig reconciliations happened",
		Aggregation: ocview.LastValue(),
	}

	hierReconcileConcurrentView = &ocview.View{
		Name:        "hnc/reconcilers/hierconfig/concurrent_peak",
		Measure:     hierConfigReconcileConcurrent,
		Description: "The peak concurrent HierConfig reconciliations happened in the past 60s, which is also the minimum Stackdriver reporting period and the one we're using",
		Aggregation: ocview.LastValue(),
	}

	hierWritesView = &ocview.View{
		Name:        "hnc/reconcilers/hierconfig/hierconfig_writes_total",
		Measure:     hierConfigWritesTotal,
		Description: "The number of HierConfig writes happened during HierConfig reconciliations",
		Aggregation: ocview.LastValue(),
	}

	namespaceWritesView = &ocview.View{
		Name:        "hnc/reconcilers/hierconfig/namespace_writes_total",
		Measure:     namespaceWritesTotal,
		Description: "The number of namespace writes happened during HierConfig reconciliations",
		Aggregation: ocview.LastValue(),
	}

	objectReconcileTotalView = &ocview.View{
		Name:        "hnc/reconcilers/object/total",
		Measure:     objectReconcileTotal,
		Description: "The total number of object reconciliations happened",
		Aggregation: ocview.LastValue(),
		TagKeys:     []tag.Key{KeyGroupKind},
	}

	objectReconcileConcurrentView = &ocview.View{
		Name:        "hnc/reconcilers/object/concurrent_peak",
		Measure:     objectReconcileConcurrent,
		Description: "The peak concurrent object reconciliations happened in the past 60s, which is also the minimum Stackdriver reporting period and the one we're using",
		Aggregation: ocview.LastValue(),
		TagKeys:     []tag.Key{KeyGroupKind},
	}

	objectWritesView = &ocview.View{
		Name:        "hnc/reconcilers/object/object_writes_total",
		Measure:     objectWritesTotal,
		Description: "The number of object writes happened during object reconciliations",
		Aggregation: ocview.LastValue(),
		TagKeys:     []tag.Key{KeyGroupKind},
	}
)

// periodicPeak contains periodic peaks for concurrent reconciliations.
type periodicPeak struct {
	// Lock is required for concurrent writes.
	lock                          sync.Mutex
	concurrentHierConfigReconcile counter
	concurrentObjectReconcile     map[schema.GroupKind]counter
}

var peak periodicPeak

func startRecordingMetrics() {
	log := ctrl.Log.WithName("metricsRecorder")

	// Register the views. It is imperative that this step exists,
	// otherwise recorded metrics will be dropped and never exported.
	if err := ocview.Register(
		hierReconcileTotalView,
		hierReconcileConcurrentView,
		hierWritesView,
		namespaceWritesView,
		objectReconcileTotalView,
		objectReconcileConcurrentView,
		objectWritesView,
	); err != nil {
		log.Error(err, "Failed to register the views")
	}

	// Start a new thread to record periodic peak.
	go recordPeakConcurrentReconciles()
}

// recordMetric records a measurement to a predefined measure without any specific tags.
func recordMetric(m counter, ms *ocstats.Int64Measure) {
	ocstats.Record(context.Background(), ms.M(int64(m)))
}

// recordTagMetric inserts a tag to the context before recording a metric.
// The tag is used to group and filter collected metrics later on.
func recordTagMetric(m counter, ms *ocstats.Int64Measure, k tag.Key, v string) {
	ctx, _ := tag.New(context.Background(), tag.Insert(k, v))
	ocstats.Record(ctx, ms.M(int64(m)))
}

func recordPeakConcurrentReconciles() {
	for {
		// This runs forever. It records and resets the peakConcurrent_ values every 1 minute,
		// which is the same as the Stackdriver exporter's reporting interval.
		time.Sleep(ReportingInterval)

		// Only lock peak during reads and writes and not during the sleeping period.
		peak.lock.Lock()
		recordMetric(peak.concurrentHierConfigReconcile, hierConfigReconcileConcurrent)
		peak.concurrentHierConfigReconcile = 0

		for gk, _ := range peak.concurrentObjectReconcile {
			recordTagMetric(peak.concurrentObjectReconcile[gk], objectReconcileConcurrent, KeyGroupKind, gk.String())
			peak.concurrentObjectReconcile[gk] = 0
		}
		peak.lock.Unlock()
	}
}
