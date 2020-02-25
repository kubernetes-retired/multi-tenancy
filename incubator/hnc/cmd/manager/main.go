/*

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

package main

import (
	"flag"
	"os"

	"contrib.go.opencensus.io/exporter/prometheus"
	"contrib.go.opencensus.io/exporter/stackdriver"
	prom "github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/stats/view"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	// +kubebuilder:scaffold:imports

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/reconcilers"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/stats"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/validators"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		metricsAddr          string
		maxReconciles        int
		enableLeaderElection bool
		novalidation         bool
		debugLogs            bool
		testLog              bool
		qps                  int
	)
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&novalidation, "novalidation", false, "Disables validating webhook")
	flag.BoolVar(&debugLogs, "debug-logs", false, "Shows verbose logs in a human-friendly format.")
	flag.BoolVar(&testLog, "enable-test-log", false, "Enables test log.")
	flag.IntVar(&maxReconciles, "max-reconciles", 1, "Number of concurrent reconciles to perform.")
	flag.IntVar(&qps, "apiserver-qps-throttle", 50, "The maximum QPS to the API server.")
	flag.Parse()

	// Enable OpenCensus exporters to export metrics
	// to Stackdriver Monitoring.
	// Exporters use Application Default Credentials to authenticate.
	// See https://developers.google.com/identity/protocols/application-default-credentials
	// for more details.
	exporter, err := stackdriver.NewExporter(stackdriver.Options{
		// Stackdriver’s minimum stats reporting period must be >= 60 seconds.
		// https://opencensus.io/exporters/supported-exporters/go/stackdriver/
		ReportingInterval: stats.ReportingInterval,
	})
	if err != nil {
		setupLog.Error(err, "cannot create Stackdriver exporter")
		os.Exit(1)
	}
	// Flush must be called before main() exits to ensure metrics are recorded.
	defer exporter.Flush()

	if err := exporter.StartMetricsExporter(); err != nil {
		setupLog.Error(err, "cannot start StackDriver metric exporter")
		os.Exit(1)
	}
	defer exporter.StopMetricsExporter()

	prom.DefaultRegisterer = metrics.Registry
	promExporter, err := prometheus.NewExporter(prometheus.Options{Registry: metrics.Registry})
	view.RegisterExporter(promExporter)

	ctrl.SetLogger(zap.Logger(debugLogs))

	cfg := ctrl.GetConfigOrDie()
	cfg.QPS = float32(qps)
	// By default, Burst is about 2x QPS, but since HNC's "bursts" can last for ~minutes
	// we need to raise the QPS param to be much higher than we ordinarily would. As a
	// result, doubling this higher threshold is probably much too high, so lower it to a more
	// reasonable number.
	//
	// TODO: Better understand the behaviour of Burst, and consider making it equal to QPS if
	// it turns out to be harmful.
	cfg.Burst = int(cfg.QPS * 1.5)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if testLog {
		stats.StartLoggingActivity()
	}

	// Create all reconciling controllers
	f := forest.NewForest()
	setupLog.Info("Creating controllers", "maxReconciles", maxReconciles)
	if err := reconcilers.Create(mgr, f, maxReconciles); err != nil {
		setupLog.Error(err, "cannot create controllers")
		os.Exit(1)
	}

	// Create validating admission controllers
	if !novalidation {
		// Create webhook for Hierarchy
		setupLog.Info("Registering validating webhook (won't work when running locally; use --novalidation)")
		mgr.GetWebhookServer().Register(validators.HierarchyServingPath, &webhook.Admission{Handler: &validators.Hierarchy{
			Log:    ctrl.Log.WithName("validators").WithName("Hierarchy"),
			Forest: f,
		}})

		// Create webhooks for managed objects
		mgr.GetWebhookServer().Register(validators.ObjectsServingPath, &webhook.Admission{Handler: &validators.Object{
			Log:    ctrl.Log.WithName("validators").WithName("Object"),
			Forest: f,
		}})
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
