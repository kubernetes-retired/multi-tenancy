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

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/vn-agent/config"
)

// InitializeTLS checks for a configured TLSCertFile and TLSPrivateKeyFile: if unspecified a new self-signed
// certificate and key file are generated.
func InitializeTLS(f *config.Flags) (*config.TLSOptions, error) {
	if f.TLSCertFile == "" && f.TLSPrivateKeyFile == "" {
		f.TLSCertFile = path.Join(f.CertDirectory, "vn.crt")
		f.TLSPrivateKeyFile = path.Join(f.CertDirectory, "vn.key")

		canReadCertAndKey, err := certutil.CanReadCertAndKey(f.TLSCertFile, f.TLSPrivateKeyFile)
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

			if err := certutil.WriteCert(f.TLSCertFile, cert); err != nil {
				return nil, err
			}

			if err := keyutil.WriteKey(f.TLSPrivateKeyFile, key); err != nil {
				return nil, err
			}

			klog.Infof("Using self-signed cert (%s, %s)", f.TLSCertFile, f.TLSPrivateKeyFile)
		}
	}

	tlsOptions := &config.TLSOptions{
		CertFile: f.TLSCertFile,
		KeyFile:  f.TLSPrivateKeyFile,
	}

	return tlsOptions, nil
}
