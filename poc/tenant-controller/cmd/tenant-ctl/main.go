// Copyright 2017 The Kubernetes Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	tenantsv1alpha "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/apis/tenants/v1alpha1"
	tenantsclient "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/clients/tenants/clientset/v1alpha1"
	tenantsinformers "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/clients/tenants/informers/externalversions"
	tenants "sigs.k8s.io/multi-tenancy/poc/tenant-controller/pkg/controllers/tenants"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var (
	masterURL  string
	kubeconfig = os.Getenv("KUBECONFIG")
)

const (
	defaultResyncInterval = time.Duration(0)
)

func init() {
	flag.StringVar(&masterURL, "master", masterURL, "The URL of the Kubernetes API server.")
	flag.StringVar(&kubeconfig, "kubeconfig", kubeconfig, "Path to kubeconfig file.")
}

func main() {
	flag.Parse()

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		glog.Fatalf("building kubeconfig: %v", err)
	}

	tenantsClient, err := tenantsclient.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("create tenants client: %v", err)
	}

	tenantsInformerFactory := tenantsinformers.NewSharedInformerFactory(tenantsClient, defaultResyncInterval)

	tenantsv1alpha.AddToScheme(scheme.Scheme)

	tenantsCtl := tenants.NewController(tenantsClient, tenantsInformerFactory)

	daemonCtx, cancelFn := context.WithCancel(context.TODO())
	sigCh, errCh := make(chan os.Signal, 1), make(chan error, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		// the first signal notifies cancels the context.
		cancelFn()
		<-sigCh
		// the second signal forcibly terminate the process.
		os.Exit(1)
	}()

	go tenantsInformerFactory.Start(daemonCtx.Done())
	go func() {
		errCh <- tenantsCtl.Run(daemonCtx)
	}()

	if err = <-errCh; err != nil {
		glog.Fatalf("controller error: %v", err)
	}
}
