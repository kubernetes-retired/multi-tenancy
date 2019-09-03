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

package config

import (
	"crypto/tls"

	kubeletclient "k8s.io/kubernetes/pkg/kubelet/client"
)

// Flags holds the config from command line.
type Flags struct {
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

	// KubeletConfig
	KubeletConfig kubeletclient.KubeletClientConfig
}

// TLSOptions holds the TLS options.
type TLSOptions struct {
	// CertFile is a cert file for TLS
	CertFile string
	// KeyFile is a key file for TLS
	KeyFile string
}

// Config holds the config of the server.
type Config struct {
	KubeletClientCert tls.Certificate
	KubeletServerHost string
}
