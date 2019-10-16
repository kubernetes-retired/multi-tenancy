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
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"net"
	"time"

	"github.com/golang/glog"
	"k8s.io/client-go/util/cert"
	keyutil "k8s.io/client-go/util/keyutil"

	tenancyv1alpha1 "github.com/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
)

const (
	DefaultClusterDomain = "cluster.local"
	neverExpireDuration  = time.Hour * 24 * 365 * 10
)

const (
	// RSAPrivateKeyBlockType is a possible value for pem.Block.Type.
	RSAPrivateKeyBlockType = "RSA PRIVATE KEY"
	// PublicKeyBlockType is a possible value for pem.Block.Type.
	PublicKeyBlockType = "PUBLIC KEY"
	// CertificateBlockType is a possible value for pem.Block.Type.
	CertificateBlockType = "CERTIFICATE"
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

type CertConfig struct {
	CommonName     string
	Organization   []string
	AltNames       cert.AltNames
	Usages         []x509.ExtKeyUsage
	ExpireDuration time.Duration
}

func genCertConfigIPs(gatewayIP string) []net.IP {
	IPs := []net.IP{}
	if gatewayIP != "" {
		glog.Infof("append gatewayIP (%s) to apiserver certificate", gatewayIP)
		IPs = append(IPs, net.ParseIP(gatewayIP))
	}
	return IPs
}

// newCertificateAuthority creates a self-signed CA crt and key pair
// based on certCfg
func NewCertificateAuthority(certCfg *cert.Config) (*CrtKeyPair, error) {
	key, err := newPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("fail to create private key: %v", err)
	}

	cert, err := cert.NewSelfSignedCACert(*certCfg, key)
	if err != nil {
		return nil, fmt.Errorf("fail to create self-signed certificate: %v", err)
	}

	return &CrtKeyPair{cert, key}, nil
}

// NewSignedCert creates a signed certificate using caCert and key
func NewSignedCert(cfg CertConfig, key *rsa.PrivateKey, caCrt *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate, error) {
	serial, err := cryptorand.Int(cryptorand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, err
	}
	if len(cfg.CommonName) == 0 {
		return nil, errors.New("must specify a CommonName")
	}
	if len(cfg.Usages) == 0 {
		return nil, errors.New("must specify at least one ExtKeyUsage")
	}

	expireDuration := cfg.ExpireDuration
	if expireDuration < 0 {
		expireDuration = neverExpireDuration
	}

	certTmpl := x509.Certificate{
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		DNSNames:     cfg.AltNames.DNSNames,
		IPAddresses:  cfg.AltNames.IPs,
		SerialNumber: serial,
		NotBefore:    caCrt.NotBefore,
		NotAfter:     time.Now().Add(expireDuration).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  cfg.Usages,
	}
	certDERBytes, err := x509.CreateCertificate(cryptorand.Reader, &certTmpl, caCrt, key.Public(), caKey)
	if err != nil {
		return nil, err
	}
	return x509.ParseCertificate(certDERBytes)
}

func NewCertAndKey(cfg CertConfig, caCrt *x509.Certificate, caKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey, error) {
	key, err := newPrivateKey()
	if err != nil {
		return nil, nil, fmt.Errorf("unable to create private key [%v]", err)
	}

	cert, err := NewSignedCert(cfg, key, caCrt, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to sign certificate [%v]", err)
	}

	return cert, key, nil
}

// newPrivateKey creates an RSA private key
func newPrivateKey() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(cryptorand.Reader, 2048)
}

// NewAPIServerCertAndKey creates crt and key for apiserver using ca.
func NewAPIServerCrtAndKey(ca *CrtKeyPair, vc *tenancyv1alpha1.Virtualcluster, apiserverDomain string) (*CrtKeyPair, error) {
	clusterDomain := DefaultClusterDomain
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
		// TODO add ips for external users to access apiserver
		// IPs: IPs,
	}

	certCfg := CertConfig{
		CommonName:     "kube-apiserver",
		AltNames:       *altNames,
		Usages:         []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		ExpireDuration: neverExpireDuration,
	}

	apiCert, apiKey, err := NewCertAndKey(certCfg, ca.Crt, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("fail to create apiserver crt and key: %v", err)
	}

	return &CrtKeyPair{apiCert, apiKey}, nil
}

// NewAPIServerKubeletClientCertAndKey generate certificate for the apiservers to connect to the kubelets securely, signed by the given CA.
func NewAPIServerKubeletClientCertAndKey(ca *CrtKeyPair) (*x509.Certificate, *rsa.PrivateKey, error) {

	config := CertConfig{
		CommonName:     "kube-apiserver-kubelet-client",
		Organization:   []string{"system:masters"},
		Usages:         []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		ExpireDuration: neverExpireDuration,
	}
	apiClientCert, apiClientKey, err := NewCertAndKey(config, ca.Crt, ca.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("failure while creating API server kubelet client key and certificate: %v", err)
	}

	return apiClientCert, apiClientKey, nil
}

// NewEtcdServerCrtAndKey creates new crt and key pair using ca for etcd
func NewEtcdServerCrtAndKey(ca *CrtKeyPair, etcdDomain string) (*CrtKeyPair, error) {

	// create AltNames with defaults DNSNames/IPs
	altNames := &cert.AltNames{
		DNSNames: []string{etcdDomain},
	}

	config := CertConfig{
		CommonName:     "kube-etcd",
		AltNames:       *altNames,
		Usages:         []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		ExpireDuration: neverExpireDuration,
	}
	etcdServerCert, etcdServerKey, err := NewCertAndKey(config, ca.Crt, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("fail to create etcd crt and key: %v", err)
	}

	return &CrtKeyPair{etcdServerCert, etcdServerKey}, nil
}

// NewEtcdHealthcheckClientCertAndKey generate certificate for liveness probes to healthcheck etcd, signed by the given CA.
func NewEtcdHealthcheckClientCertAndKey(ca *CrtKeyPair) (*x509.Certificate, *rsa.PrivateKey, error) {

	config := CertConfig{
		CommonName:     "kube-etcd-healthcheck-client",
		Organization:   []string{"system:masters"},
		Usages:         []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		ExpireDuration: neverExpireDuration,
	}
	etcdHealcheckClientCert, etcdHealcheckClientKey, err := NewCertAndKey(config, ca.Crt, ca.Key)
	if err != nil {
		return nil, nil, fmt.Errorf("failure while creating etcd healthcheck client key and certificate: %v", err)
	}

	return etcdHealcheckClientCert, etcdHealcheckClientKey, nil
}

// NewServiceAccountSigningKey generate public/private key pairs for signing service account tokens.
func NewServiceAccountSigningKey() (*rsa.PrivateKey, error) {

	// The key does NOT exist, let's generate it now
	saSigningKey, err := newPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failure while creating service account token signing key: %v", err)
	}

	return saSigningKey, nil
}

// NewFrontProxyClientCertAndKey creates crt and key for proxy client using ca.
func NewFrontProxyClientCertAndKey(ca *CrtKeyPair) (*CrtKeyPair, error) {

	config := CertConfig{
		CommonName:     "front-proxy-client",
		Usages:         []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		ExpireDuration: neverExpireDuration,
	}
	frontProxyClientCert, frontProxyClientKey, err := NewCertAndKey(config, ca.Crt, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("fail to create crt and key for front-proxy: %v", err)
	}

	return &CrtKeyPair{frontProxyClientCert, frontProxyClientKey}, nil
}

func NewClientCrtAndKey(user string, ca *CrtKeyPair) (*CrtKeyPair, error) {
	crt, key, err := NewCertAndKey(CertConfig{
		CommonName:     user,
		Usages:         []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		ExpireDuration: neverExpireDuration,
	}, ca.Crt, ca.Key)

	if err != nil {
		return nil, err
	}

	return &CrtKeyPair{crt, key}, nil
}

// EncodeCertPEM returns PEM-endcoded certificate data
func EncodeCertPEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  CertificateBlockType,
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

// ParseCertPEM decodes PEM-encoded data
func ParseCertPEM(data []byte) (*x509.Certificate, error) {
	certs, err := cert.ParseCertsPEM(data)
	if nil != err {
		return nil, err
	}

	return certs[0], nil
}

func ParsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	// Parse the private key from a file
	privKey, err := keyutil.ParsePrivateKeyPEM(data)
	if err != nil {
		return nil, err
	}
	// Allow RSA format only
	var key *rsa.PrivateKey
	switch k := privKey.(type) {
	case *rsa.PrivateKey:
		key = k
	default:
		return nil, fmt.Errorf("the private key data isn't in RSA format")
	}

	return key, nil
}

// EncodePrivateKeyPEM returns PEM-encoded private key data
func EncodePrivateKeyPEM(key *rsa.PrivateKey) []byte {
	block := pem.Block{
		Type:  RSAPrivateKeyBlockType,
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return pem.EncodeToMemory(&block)
}

// EncodePublicKeyPEM returns PEM-encoded public data
func EncodePublicKeyPEM(key *rsa.PublicKey) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return []byte{}, err
	}
	block := pem.Block{
		Type:  PublicKeyBlockType,
		Bytes: der,
	}
	return pem.EncodeToMemory(&block), nil
}

func ParsePublicKey(data []byte) (*rsa.PublicKey, error) {
	// Parse the private key from a file
	pubKeys, err := keyutil.ParsePublicKeysPEM(data)
	if err != nil {
		return nil, err
	}

	// Allow RSA format only
	var key *rsa.PublicKey
	switch k := pubKeys[0].(type) {
	case *rsa.PublicKey:
		key = k
	default:
		return nil, fmt.Errorf("the public key data isn't in RSA format")
	}

	return key, nil
}
