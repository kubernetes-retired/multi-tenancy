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

package certificate

import (
	"fmt"
	"os"
	"path"

	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/config"
)

// InitializeTLS checks for a configured TLSCertFile and TLSPrivateKeyFile: if unspecified a new self-signed
// certificate and key file are generated.
func InitializeTLS(certDirectory, certFile, privateKeyFile, suffix string) (*config.TLSOptions, error) {
	if certFile == "" && privateKeyFile == "" {
		certFile = path.Join(certDirectory, fmt.Sprintf("%s.crt", suffix))
		privateKeyFile = path.Join(certDirectory, fmt.Sprintf("%s.key", suffix))

		canReadCertAndKey, err := certutil.CanReadCertAndKey(certFile, privateKeyFile)
		if err != nil {
			return nil, err
		}

		if !canReadCertAndKey {
			hostName, err := os.Hostname()
			if err != nil {
				return nil, fmt.Errorf("couldn't determine hostname: %v", err)
			}
			cert, key, err := certutil.GenerateSelfSignedCertKey(hostName, nil, nil)
			if err != nil {
				return nil, fmt.Errorf("unable to generate self signed cert: %v", err)
			}

			if err := certutil.WriteCert(certFile, cert); err != nil {
				return nil, err
			}

			if err := keyutil.WriteKey(privateKeyFile, key); err != nil {
				return nil, err
			}

			klog.Infof("Using self-signed cert (%s, %s)", certFile, privateKeyFile)
		}
	}

	tlsOptions := &config.TLSOptions{
		CertFile: certFile,
		KeyFile:  privateKeyFile,
	}

	return tlsOptions, nil
}
