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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"k8s.io/klog"
	"net/http"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/component-base/logs"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/certificate"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/server"
)

func main() {
	var cfg config.Flags

	cmd := NewCommand(cfg)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

// NewCommand creates a new top-level command.
func NewCommand(c config.Flags) *cobra.Command {
	cmd := &cobra.Command{
		Use: "vn-agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(&c)
		},
	}

	installFlags(cmd.Flags(), &c)
	return cmd
}

func installFlags(flags *pflag.FlagSet, c *config.Flags) {
	flags.StringVar(&c.ClientCAFile, "client-ca-file", c.ClientCAFile, "kube config file to use for connecting to the Kubernetes API server")
	flags.StringVar(&c.CertDirectory, "cert-dir", c.CertDirectory, "CertDirectory is the directory where the TLS certs are located")
	flags.StringVar(&c.TLSCertFile, "tls-cert-file", c.TLSCertFile, "TLSCertFile is the file containing x509 Certificate for HTTPS")
	flags.StringVar(&c.TLSPrivateKeyFile, "tls-private-key-file", c.TLSPrivateKeyFile, "TLSPrivateKeyFile is the file containing x509 private key matching tlsCertFile")
	flags.UintVar(&c.Port, "port", 10550, "Port is the server listen on")

	flags.StringVar(&c.KubeletConfig.CertFile, "kubelet-client-certificate", c.KubeletConfig.CertFile, "Path to a client cert file for TLS")
	flags.StringVar(&c.KubeletConfig.KeyFile, "kubelet-client-key", c.KubeletConfig.KeyFile, "Path to a client key file for TLS")
	flags.UintVar(&c.KubeletConfig.Port, "kubelet-port", 10250, "Kubelet security port")
}

func run(c *config.Flags) error {
	kubeletClientCertPair, err := tls.LoadX509KeyPair(c.KubeletConfig.CertFile, c.KubeletConfig.KeyFile)
	if err != nil {
		return errors.Wrapf(err, "failed to load kubelet tls config")
	}

	handler, err := server.NewServer(&config.Config{
		KubeletClientCert: kubeletClientCertPair,
		KubeletServerHost: fmt.Sprintf("https://127.0.0.1:%v", c.KubeletConfig.Port),
	})
	if err != nil {
		return errors.Wrapf(err, "create server")
	}

	s := &http.Server{
		Addr:    fmt.Sprintf(":%d", c.Port),
		Handler: handler,
		TLSConfig: &tls.Config{
			ClientAuth: tls.RequestClientCert,
		},
	}

	if c.ClientCAFile != "" {
		clientCAs, err := certutil.CertsFromFile(c.ClientCAFile)
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

	tlsConfig, err := certificate.InitializeTLS(c)
	if err != nil {
		return errors.Wrapf(err, "failed to initial tls config")
	}

	logs.InitLogs()
	defer logs.FlushLogs()

	klog.Infof("server listen on %s", s.Addr)
	klog.Infof("config %+v", c)
	klog.Fatal(s.ListenAndServeTLS(tlsConfig.CertFile, tlsConfig.KeyFile))

	return nil
}
