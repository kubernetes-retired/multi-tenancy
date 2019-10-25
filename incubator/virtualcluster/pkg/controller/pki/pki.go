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

package pki

import (
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"

	"k8s.io/client-go/util/cert"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"

	tenancyv1alpha1 "github.com/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
)

const (
	defaultClusterDomain = "cluster.local"
)

type CrtKeyPair struct {
	Crt *x509.Certificate
	Key *rsa.PrivateKey
}

type ClusterCAGroup struct {
	RootCA                   *CrtKeyPair
	APIServer                *CrtKeyPair
	ETCD                     *CrtKeyPair
	CtrlMgrKbCfg             string // the kubeconfig used by controller-manager
	AdminKbCfg               string // the kubeconfig used by admin user
	ServiceAccountPrivateKey *rsa.PrivateKey
}

// NewAPIServerCertAndKey creates crt and key for apiserver using ca.
func NewAPIServerCrtAndKey(ca *CrtKeyPair, vc *tenancyv1alpha1.Virtualcluster, apiserverDomain string) (*CrtKeyPair, error) {
	clusterDomain := defaultClusterDomain
	if vc.Spec.ClusterDomain != "" {
		clusterDomain = vc.Spec.ClusterDomain
	}

	// create AltNames with defaults DNSNames/IPs
	altNames := &cert.AltNames{
		DNSNames: []string{
			"kubernetes",
			"kubernetes.default",
			"kubernetes.default.svc",
			fmt.Sprintf("kubernetes.default.svc.%s", clusterDomain),
			apiserverDomain,
			// add virtual cluster name (i.e. namespace) for vn-agent
			vc.Name,
		},
	}

	config := &cert.Config{
		CommonName: "kube-apiserver",
		AltNames:   *altNames,
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}

	apiCert, apiKey, err := pkiutil.NewCertAndKey(ca.Crt, ca.Key, config)
	if err != nil {
		return nil, fmt.Errorf("fail to create apiserver crt and key: %v", err)
	}

	rsaKey, ok := apiKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("fail to assert rsa private key")
	}

	return &CrtKeyPair{apiCert, rsaKey}, nil
}

// NewAPIServerKubeletClientCertAndKey creates certificate for the apiservers to connect to the
// kubelets securely, signed by the ca.
func NewAPIServerKubeletClientCertAndKey(ca *CrtKeyPair) (*x509.Certificate, *rsa.PrivateKey, error) {
	config := &cert.Config{
		CommonName:   "kube-apiserver-kubelet-client",
		Organization: []string{"system:masters"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	apiClientCert, apiClientKey, err := pkiutil.NewCertAndKey(ca.Crt, ca.Key, config)
	if err != nil {
		return nil, nil, fmt.Errorf("failure while creating API server kubelet client key and certificate: %v", err)
	}

	rsaKey, ok := apiClientKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("fail to assert rsa private key")
	}

	return apiClientCert, rsaKey, nil
}

// NewEtcdServerCrtAndKey creates new crt-key pair using ca for etcd
func NewEtcdServerCrtAndKey(ca *CrtKeyPair, etcdDomains []string) (*CrtKeyPair, error) {
	// create AltNames with defaults DNSNames/IPs
	altNames := &cert.AltNames{
		DNSNames: etcdDomains,
		IPs:      []net.IP{net.ParseIP("127.0.0.1")},
	}

	config := &cert.Config{
		CommonName: "kube-etcd",
		AltNames:   *altNames,
		// all peers will use this crt-key pair as well
		Usages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	etcdServerCert, etcdServerKey, err := pkiutil.NewCertAndKey(ca.Crt, ca.Key, config)
	if err != nil {
		return nil, fmt.Errorf("fail to create etcd crt and key: %v", err)
	}

	rsaKey, ok := etcdServerKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("fail to assert rsa private key")
	}

	return &CrtKeyPair{etcdServerCert, rsaKey}, nil
}

// NewEtcdHealthcheckClientCertAndKey creates certificate for liveness probes to healthcheck etcd,
// signed by the given ca.
func NewEtcdHealthcheckClientCertAndKey(ca *CrtKeyPair) (*x509.Certificate, *rsa.PrivateKey, error) {

	config := &cert.Config{
		CommonName:   "kube-etcd-healthcheck-client",
		Organization: []string{"system:masters"},
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	etcdHealcheckClientCert, etcdHealcheckClientKey, err := pkiutil.NewCertAndKey(ca.Crt, ca.Key, config)
	if err != nil {
		return nil, nil, fmt.Errorf("failure while creating etcd healthcheck client key and certificate: %v", err)
	}

	rsaKey, ok := etcdHealcheckClientKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("fail to assert rsa private key")
	}

	return etcdHealcheckClientCert, rsaKey, nil
}

// NewServiceAccountSigningKey creates rsa key for signing service account tokens.
func NewServiceAccountSigningKey() (*rsa.PrivateKey, error) {

	// The key does NOT exist, let's generate it now
	saSigningKey, err := newPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failure while creating service account token signing key: %v", err)
	}

	return saSigningKey, nil
}

// NewFrontProxyClientCertAndKey creates crt-key pair for proxy client using ca.
func NewFrontProxyClientCertAndKey(ca *CrtKeyPair) (*CrtKeyPair, error) {

	config := &cert.Config{
		CommonName: "front-proxy-client",
		Usages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	frontProxyClientCert, frontProxyClientKey, err := pkiutil.NewCertAndKey(ca.Crt, ca.Key, config)
	if err != nil {
		return nil, fmt.Errorf("fail to create crt and key for front-proxy: %v", err)
	}
	rsaKey, ok := frontProxyClientKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("fail to assert rsa private key")
	}
	return &CrtKeyPair{frontProxyClientCert, rsaKey}, nil
}

// NewClientCrtAndKey creates crt-key pair for client
func NewClientCrtAndKey(user string, ca *CrtKeyPair, groups []string) (*CrtKeyPair, error) {
	config := &cert.Config{
		CommonName:   user,
		Organization: groups,
		Usages:       []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}

	crt, key, err := pkiutil.NewCertAndKey(ca.Crt, ca.Key, config)
	if err != nil {
		return nil, err
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("fail to assert rsa private key")
	}

	return &CrtKeyPair{crt, rsaKey}, nil
}

// EncodePrivateKeyPEM returns PEM-encoded private key data
func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  pkiutil.RSAPrivateKeyBlockType,
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(&block)
}

// newPrivateKey creates an RSA private key
func newPrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(cryptorand.Reader, 2048)
}
