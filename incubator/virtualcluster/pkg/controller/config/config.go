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

type Config struct {
	MasterProvisioner       string
	NativeProvisionerConfig *NativeProvisionerConfig
}

type NativeProvisionerConfig struct {
	// RootCACertFile If set, this root certificate authority will be used to sign tenant's certificate.
	// This must be a valid PEM-encoded CA bundle.
	RootCACertFile string
	// RootCAKeyFile is the file containing x509 private key matching the certFile.
	RootCAKeyFile string
}

func NewVCControllerConfig() *Config {
	return &Config{
		NativeProvisionerConfig: &NativeProvisionerConfig{},
	}
}
