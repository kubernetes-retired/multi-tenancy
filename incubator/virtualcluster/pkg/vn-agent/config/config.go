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
)

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
