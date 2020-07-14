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

	// Change to use v1 when we only need to support 1.17 and higher kubernetes versions.
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	// +kubebuilder:scaffold:imports

	v1a1 "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
	v1a2 "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/reconcilers"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/stats"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/validators"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = zap.New().WithName("setup")
)

var (
	metricsAddr          string
	maxReconciles        int
	enableLeaderElection bool
	leaderElectionId     string
	novalidation         bool
	debugLogs            bool
	testLog              bool
	internalCert         bool
	qps                  int
	webhookServerPort    int
)

func init() {
	setupLog.Info("Starting main.go:init()")
	defer setupLog.Info("Finished main.go:init()")
	_ = clientgoscheme.AddToScheme(scheme)

	_ = v1a1.AddToScheme(scheme)
	_ = v1a2.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	setupLog.Info("Parsing flags")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionId, "leader-election-id", "controller-leader-election-helper",
		"Leader election id determines the name of the configmap that leader election will use for holding the leader lock.")
	flag.BoolVar(&novalidation, "novalidation", false, "Disables validating webhook")
	flag.BoolVar(&debugLogs, "debug-logs", false, "Shows verbose logs in a human-friendly format.")
	flag.BoolVar(&testLog, "enable-test-log", false, "Enables test log.")
	flag.BoolVar(&internalCert, "enable-internal-cert-management", false, "Enables internal cert management.")
	flag.IntVar(&maxReconciles, "max-reconciles", 1, "Number of concurrent reconciles to perform.")
	flag.IntVar(&qps, "apiserver-qps-throttle", 50, "The maximum QPS to the API server.")
	flag.BoolVar(&stats.SuppressObjectTags, "suppress-object-tags", true, "If true, suppresses the kinds of object metrics to reduce metric cardinality.")
	flag.IntVar(&webhookServerPort, "webhook-server-port", 443, "The port that the webhook server serves at.")
	flag.Parse()

	// Enable OpenCensus exporters to export metrics
	// to Stackdriver Monitoring.
	// Exporters use Application Default Credentials to authenticate.
	// See https://developers.google.com/identity/protocols/application-default-credentials
	// for more details.
	setupLog.Info("Creating OpenCensus->Stackdriver exporter")
	sd, err := stackdriver.NewExporter(stackdriver.Options{
		// Stackdriver’s minimum stats reporting period must be >= 60 seconds.
		// https://opencensus.io/exporters/supported-exporters/go/stackdriver/
		ReportingInterval: stats.ReportingInterval,
	})
	if err == nil {
		// Flush must be called before main() exits to ensure metrics are recorded.
		defer sd.Flush()
		err = sd.StartMetricsExporter()
		if err == nil {
			defer sd.StopMetricsExporter()
		}
	}
	if err != nil {
		setupLog.Error(err, "cannot create Stackdriver exporter")
	}

	setupLog.Info("Creating Prometheus exporter")
	prom.DefaultRegisterer = prom.DefaultRegisterer.(*prom.Registry)
	promExporter, err := prometheus.NewExporter(prometheus.Options{Registry: prom.DefaultRegisterer.(*prom.Registry)})
	if err != nil {
		setupLog.Error(err, "Cannot create Prometheus exporter")
	}
	view.RegisterExporter(promExporter)

	setupLog.Info("Configuring controller-manager")
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
		LeaderElectionID:   leaderElectionId,
		Port:               webhookServerPort,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Make sure certs are generated and valid if webhooks are enabled and internal certs are used.
	setupLog.Info("Starting certificate generation")
	setupFinished, err := validators.CreateCertsIfNeeded(mgr, novalidation, internalCert)
	if err != nil {
		setupLog.Error(err, "unable to set up cert rotation")
		os.Exit(1)
	}

	go startControllers(mgr, setupFinished)

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func startControllers(mgr ctrl.Manager, setupFinished chan struct{}) {
	setupLog.Info("Waiting for certificate generation to complete")
	// Block until the setup finishes.
	<-setupFinished

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

	// Create all validating admission controllers.
	if !novalidation {
		setupLog.Info("Registering validating webhook (won't work when running locally; use --novalidation)")
		validators.Create(mgr, f)
	}

	// Create CRD conversion webhooks.
	if err := (&v1a1.HNCConfiguration{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create CRD convension webhook", v1a1.HNCConfigSingletons)
		os.Exit(1)
	}

	setupLog.Info("All controllers started; setup complete")
}
