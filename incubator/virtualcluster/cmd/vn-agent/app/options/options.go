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

package options

import (
	"crypto/tls"
	"fmt"
	"os"

	"github.com/pkg/errors"
	cliflag "k8s.io/component-base/cli/flag"
	kubeletclient "k8s.io/kubernetes/pkg/kubelet/client"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/config"
)

// Options holds the config from command line.
type Options struct {
	// ServerOption
	ServerOption
	// KubeletOption
	KubeletOption kubeletclient.KubeletClientConfig
}

type ServerOption struct {
	// ClientCAFile is the path to a PEM-encoded certificate bundle. If set, any request presenting a client certificate
	// signed by one of the authorities in the bundle is authenticated with a username corresponding to the CommonName,
	// and groups corresponding to the Organization in the client certificate.
	ClientCAFile string
	// CertDirectory is the directory where the TLS certs are located
	CertDirectory string
	// TLSCertFile is the file containing x509 Certificate for HTTPS
	TLSCertFile string
	// TLSPrivateKeyFile is the file containing x509 private key matching tlsCertFile
	TLSPrivateKeyFile string

	// Port is the vn-agent server listening on.
	Port uint
}

func NewVnAgentOptions() (*Options, error) {
	return &Options{
		KubeletOption: kubeletclient.KubeletClientConfig{},
	}, nil
}

// Flags in command line.
func (o *Options) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}

	serverFS := fss.FlagSet("server")
	serverFS.StringVar(&o.ClientCAFile, "client-ca-file", o.ClientCAFile, "kube config file to use for connecting to the Kubernetes API server")
	serverFS.StringVar(&o.CertDirectory, "cert-dir", o.CertDirectory, "CertDirectory is the directory where the TLS certs are located")
	serverFS.StringVar(&o.TLSCertFile, "tls-cert-file", o.TLSCertFile, "TLSCertFile is the file containing x509 Certificate for HTTPS")
	serverFS.StringVar(&o.TLSPrivateKeyFile, "tls-private-key-file", o.TLSPrivateKeyFile, "TLSPrivateKeyFile is the file containing x509 private key matching tlsCertFile")
	serverFS.UintVar(&o.Port, "port", 10550, "Port is the server listen on")

	kubeletFS := fss.FlagSet("kubelet")
	kubeletFS.StringVar(&o.KubeletOption.CertFile, "kubelet-client-certificate", o.KubeletOption.CertFile, "Path to a client cert file for TLS")
	kubeletFS.StringVar(&o.KubeletOption.KeyFile, "kubelet-client-key", o.KubeletOption.KeyFile, "Path to a client key file for TLS")
	kubeletFS.UintVar(&o.KubeletOption.Port, "kubelet-port", 10250, "Kubelet security port")

	return fss
}

func fileNotExistOrEmpty(fn string) bool {
	if fn == "" {
		return true
	}
	fi, _ := os.Stat(fn)
	return fi.Size() == 0
}

// Config is the config to create a vn-agent server handler.
func (o *Options) Config() (*config.Config, *ServerOption, error) {
	// vc-kubelet-client may be a place holder that contains empty certificate and key data
	if fileNotExistOrEmpty(o.KubeletOption.CertFile) || fileNotExistOrEmpty(o.KubeletOption.KeyFile) {
		return &config.Config{KubeletClientCert: nil}, &o.ServerOption, nil
	}
	kubeletClientCertPair, err := tls.LoadX509KeyPair(o.KubeletOption.CertFile, o.KubeletOption.KeyFile)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to load kubelet tls config")
	}
	return &config.Config{
		KubeletClientCert: &kubeletClientCertPair,
		KubeletServerHost: fmt.Sprintf("https://127.0.0.1:%v", o.KubeletOption.Port),
	}, &o.ServerOption, nil
}
