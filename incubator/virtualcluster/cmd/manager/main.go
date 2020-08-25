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

package main

import (
	"flag"
	"fmt"
	"os"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/webhook"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/constants"
	logrutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/logr"
	vcmanager "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/vcmanager"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/version"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/version/verflag"
)

func main() {
	var (
		logFile                 string
		metricsAddr             string
		masterProvisioner       string
		leaderElection          bool
		leaderElectionCmName    string
		maxConcurrentReconciles int
		versionOpt              bool
		disableStacktrace       bool
		enableWebhook           bool
	)
	flag.StringVar(&metricsAddr, "metrics-addr", ":0", "The address the metric endpoint binds to.")
	flag.StringVar(&masterProvisioner, "master-prov", "native",
		"The underlying platform that will provision master for virtualcluster.")
	flag.BoolVar(&leaderElection, "leader-election", true, "If enable leaderelection for vc-manager")
	flag.StringVar(&leaderElectionCmName, "le-cm-name", "vc-manager-leaderelection-lock",
		"The name of the configmap that will be used as the resourcelook for leaderelection")
	flag.IntVar(&maxConcurrentReconciles, "num-reconciles", 10,
		"The max number reconcilers of virtualcluster controller")
	flag.StringVar(&logFile, "log-file", "", "The path of the logfile, if not set, only log to the stderr")
	flag.BoolVar(&versionOpt, "version", false, "Print the version information")
	flag.BoolVar(&disableStacktrace, "disable-stacktrace", false, "If set, the automatic stacktrace is disabled")
	flag.BoolVar(&enableWebhook, "enable-webhook", false, "If set, the virtualcluster webhook is enabled")

	flag.Parse()

	// print version information
	if versionOpt {
		fmt.Printf("VirtualCluster %s\n", verflag.GetVersion(version.Get()))
		os.Exit(0)
	}

	loggr, err := logrutil.NewLogger(logFile, disableStacktrace)
	if err != nil {
		panic(fmt.Sprintf("fail to initialize logr: %s", err))
	}
	logf.SetLogger(loggr)
	log := logf.Log.WithName("entrypoint")

	// Get a config to talk to the apiserver
	log.Info("setting up client for manager")
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to set up client config")
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	log.Info("setting up manager")
	mgrOpt := manager.Options{
		MetricsBindAddress: metricsAddr,
		LeaderElection:     leaderElection,
		LeaderElectionID:   leaderElectionCmName,
		CertDir:            constants.VirtualClusterWebhookCertDir,
		Port:               constants.VirtualClusterWebhookPort,
	}
	mgr, err := vcmanager.NewVirtualClusterManager(cfg, mgrOpt, maxConcurrentReconciles)
	if err != nil {
		log.Error(err, "unable to set up overall controller manager")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	log.Info("setting up scheme")
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "unable add APIs to scheme")
		os.Exit(1)
	}

	// Setup all Controllers
	log.Info("Setting up controller")
	if err := controller.AddToManager(mgr, masterProvisioner); err != nil {
		log.Error(err, "unable to register controllers to the manager")
		os.Exit(1)
	}

	if enableWebhook == true {
		log.Info("setting up webhooks")
		if err := webhook.AddToManager(mgr, mgrOpt.CertDir); err != nil {
			log.Error(err, "unable to register webhooks to the manager")
			os.Exit(1)
		}
	}

	// Start the Cmd
	log.Info("Starting the Cmd.")
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "unable to run the manager")
		os.Exit(1)
	}
}
