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

var (
	// SuppressObjectTags, if true, will prevent any GroupKind tags from being annotated onto object
	// metrics.
	SuppressObjectTags bool
)

// Create Measures. A measure represents a metric type to be recorded.
var (
	hierConfigReconcileTotal      = ocstats.Int64("hierconfig_reconcile_total", "The total number of HierConfig reconciliations happened", "reconciliations")
	hierConfigReconcileConcurrent = ocstats.Int64("hierconfig_reconcile_concurrent_peak", "The peak concurrent HierConfig reconciliations happened in the last reporting period", "reconciliations")
	hierConfigWritesTotal         = ocstats.Int64("hierconfig_writes_total", "The number of HierConfig writes happened during HierConfig reconciliations", "writes")
	namespaceWritesTotal          = ocstats.Int64("namespace_writes_total", "The number of namespace writes happened during HierConfig reconciliations", "writes")
	objectReconcileTotal          = ocstats.Int64("object_reconcile_total", "The total number of object reconciliations happened", "reconciliations")
	objectReconcileConcurrent     = ocstats.Int64("object_reconcile_concurrent_peak", "The peak concurrent object reconciliations happened in the last reporting period", "reconciliations")
	objectWritesTotal             = ocstats.Int64("object_writes_total", "The number of object writes happened during object reconciliations", "writes")
	namespaceConditions           = ocstats.Int64("namespace_conditions", "The number of namespaces with conditions", "conditions")
	objectOverwritesTotal         = ocstats.Int64("object_overwrites_total", "The number of overwritten objects", "overwrites")
)

// Create Tags. Tags are used to group and filter collected metrics later on.
// Create a GroupKind Tag for metrics of object reconcilers for different GroupKind.
var KeyGroupKind, _ = tag.NewKey("GroupKind")

// KeyNamespaceConditionType is the type of the namespace condition. The values
// could be "ActivitiesHalted" or "BadConfiguration".
var KeyNamespaceConditionType, _ = tag.NewKey("Condition")

// KeyNamespaceConditionReason indicates the reason of the namespace condition.
// The values could be "InCycle", "ParentMissing", etc.
var KeyNamespaceConditionReason, _ = tag.NewKey("Reason")

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
		Name:        "hnc/reconcilers/object/writes_total",
		Measure:     objectWritesTotal,
		Description: "The number of object writes happened during object reconciliations",
		Aggregation: ocview.LastValue(),
		TagKeys:     []tag.Key{KeyGroupKind},
	}

	namespaceConditionsView = &ocview.View{
		Name:        "hnc/namespace_conditions",
		Measure:     namespaceConditions,
		Description: "The number of namespaces with conditions",
		Aggregation: ocview.LastValue(),
		TagKeys:     []tag.Key{KeyNamespaceConditionType, KeyNamespaceConditionReason},
	}

	objectOverwritesTotalView = &ocview.View{
		Name:        "hnc/reconcilers/object/overwrites_total",
		Measure:     objectOverwritesTotal,
		Description: "The number of overwritten objects",
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
		namespaceConditionsView,
		objectOverwritesTotalView,
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

// recordObjectMetric records a measurement specifically associated with an object. If
// SuppressObjectTags isn't set, it also tags the measurement with the provided GroupKind.
func recordObjectMetric(m counter, ms *ocstats.Int64Measure, gk schema.GroupKind) {
	ctx := context.Background()
	if !SuppressObjectTags {
		ctx, _ = tag.New(ctx, tag.Insert(KeyGroupKind, gk.String()))
	}
	ocstats.Record(ctx, ms.M(int64(m)))
}

func RecordNamespaceCondition(tp, reason string, num int) {
	ctx, _ := tag.New(context.Background(), tag.Insert(KeyNamespaceConditionType, tp))
	ctx, _ = tag.New(ctx, tag.Insert(KeyNamespaceConditionReason, reason))
	ocstats.Record(ctx, namespaceConditions.M(int64(num)))
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
			recordObjectMetric(peak.concurrentObjectReconcile[gk], objectReconcileConcurrent, gk)
			peak.concurrentObjectReconcile[gk] = 0
		}
		peak.lock.Unlock()
	}
}
