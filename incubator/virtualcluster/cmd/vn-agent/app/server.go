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

package app

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"k8s.io/apiserver/pkg/util/term"
	certutil "k8s.io/client-go/util/cert"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/cli/globalflag"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/cmd/vn-agent/app/options"
	utilflag "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/flag"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/version/verflag"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/certificate"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/server"
)

func NewVnAgentCommand(stopChan <-chan struct{}) *cobra.Command {
	s, err := options.NewVnAgentOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	cmd := &cobra.Command{
		Use:  "vn-agent",
		Long: `The vn-agent is proxy between tenant apiserver and kubelet server on physical node.`,
		Run: func(cmd *cobra.Command, args []string) {
			verflag.PrintAndExitIfRequested()
			utilflag.PrintFlags(cmd.Flags())

			c, serverOptions, err := s.Config()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}

			if err := Run(c, serverOptions, stopChan); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		},
	}

	fs := cmd.Flags()
	namedFlagSets := s.Flags()
	verflag.AddFlags(namedFlagSets.FlagSet("global"))
	globalflag.AddGlobalFlags(namedFlagSets.FlagSet("global"), cmd.Name())

	for _, f := range namedFlagSets.FlagSets {
		fs.AddFlagSet(f)
	}
	usageFmt := "Usage:\n  %s\n"
	cols, _, _ := term.TerminalSize(cmd.OutOrStdout())
	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStderr(), namedFlagSets, cols)
		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
		cliflag.PrintSections(cmd.OutOrStdout(), namedFlagSets, cols)
	})

	return cmd
}

// Run start the vn-agent server.
func Run(c *config.Config, serverOption *options.ServerOption, stopCh <-chan struct{}) error {
	handler, err := server.NewServer(c)
	if err != nil {
		return errors.Wrapf(err, "create server")
	}

	s := &http.Server{
		Addr:    fmt.Sprintf(":%d", serverOption.Port),
		Handler: handler,
		TLSConfig: &tls.Config{
			ClientAuth: tls.RequestClientCert,
		},
	}

	if serverOption.ClientCAFile != "" {
		clientCAs, err := certutil.CertsFromFile(serverOption.ClientCAFile)
		if err != nil {
			return errors.Wrapf(err, "unable to load client CA file")
		}

		certPool := x509.NewCertPool()
		for _, cert := range clientCAs {
			certPool.AddCert(cert)
		}

		s.TLSConfig.ClientCAs = certPool
		s.TLSConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	tlsConfig, err := certificate.InitializeTLS(serverOption.CertDirectory, serverOption.TLSCertFile, serverOption.TLSPrivateKeyFile, "vn")
	if err != nil {
		return errors.Wrapf(err, "failed to initial tls config")
	}

	klog.Infof("server listen on %s", s.Addr)

	errCh := make(chan error)
	go func() {
		err := s.ListenAndServeTLS(tlsConfig.CertFile, tlsConfig.KeyFile)
		errCh <- err
	}()

	select {
	case <-stopCh:
		klog.Infof("closing server...")
		s.Close()
	case err := <-errCh:
		klog.Errorf("server listen error %v", err)
	}

	return nil
}
