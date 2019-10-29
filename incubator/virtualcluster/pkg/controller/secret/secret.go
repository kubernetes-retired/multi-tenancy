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

package secret

import (
	"crypto/rsa"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pkiutil"

	vcpki "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/pki"
)

const (
	RootCASecretName            = "root-ca"
	APIServerCASecretName       = "apiserver-ca"
	ETCDCASecretName            = "etcd-ca"
	ControllerManagerSecretName = "controller-manager-kubeconfig"
	AdminSecretName             = "admin-kubeconfig"
	ServiceAccountSecretName    = "serviceaccount-rsa"
)

// RsaKeyToSecret encapsulates rsaKey into a secret object
func RsaKeyToSecret(name, namespace string, rsaKey *rsa.PrivateKey) (*v1.Secret, error) {
	encodedPubKey, err := pkiutil.EncodePublicKeyPEM(&rsaKey.PublicKey)
	if err != nil {
		return nil, err
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			v1.TLSCertKey:       encodedPubKey,
			v1.TLSPrivateKeyKey: vcpki.EncodePrivateKeyPEM(rsaKey),
		},
	}, nil
}

// CrtKeyPairToSecret encapsulates ca/key pair ckp into a secret object
func CrtKeyPairToSecret(name, namespace string, ckp *vcpki.CrtKeyPair) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: v1.SecretTypeTLS,
		Data: map[string][]byte{
			v1.TLSCertKey:       pkiutil.EncodeCertPEM(ckp.Crt),
			v1.TLSPrivateKeyKey: vcpki.EncodePrivateKeyPEM(ckp.Key),
		},
	}
}

// KubeconfigToSecret encapsulates kubeconfig cfgContent into a secret object
func KubeconfigToSecret(name, namespace string, cfgContent string) *v1.Secret {
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Type: v1.SecretTypeOpaque,
		Data: map[string][]byte{
			name: []byte(cfgContent),
		},
	}
}
