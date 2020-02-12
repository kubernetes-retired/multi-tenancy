package stats

import (
	"context"

	ocstats "go.opencensus.io/stats"
	ocview "go.opencensus.io/stats/view"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	hierConfigReconcileTotal = ocstats.Int64("hierconfig_reconcile_total", "The number of HierConfig reconciliations happened", "times")
	hierConfigWritesTotal    = ocstats.Int64("hierconfig_writes_total", "The number of HierConfig writes happened during HierConfig reconciliations", "times")
)

var (
	hierReconcileView = &ocview.View{
		Name:        "hnc/reconcilers/hierconfig/total",
		Measure:     hierConfigReconcileTotal,
		Description: "The number of HierConfig reconciliations happened",
		Aggregation: ocview.LastValue(),
	}

	hierWritesView = &ocview.View{
		Name:        "hnc/reconcilers/hierconfig/hierconfig_writes_total",
		Measure:     hierConfigWritesTotal,
		Description: "The number of HierConfig writes happened during HierConfig reconciliations",
		Aggregation: ocview.LastValue(),
	}
)

func startRecordingMetrics() {
	log := ctrl.Log.WithName("metricsRecorder")

	// Register the view. It is imperative that this step exists,
	// otherwise recorded metrics will be dropped and never exported.
	if err := ocview.Register(hierReconcileView, hierWritesView); err != nil {
		log.Error(err, "Failed to register the views")
	}
}

func recordMetricsInt64(c counter, ms *ocstats.Int64Measure) {
	ocstats.Record(context.Background(), ms.M(int64(c)))
}
