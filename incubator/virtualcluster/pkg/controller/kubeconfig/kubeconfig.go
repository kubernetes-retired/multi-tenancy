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

package kubeconfig

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
	"text/template"

	vcpki "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/controller/pki"
)

const (
	kubeconfigTemplate = `
kind: Config
apiVersion: v1
users:
- name: {{ .username }}
  user:
    client-certificate-data: {{ .cert }}
    client-key-data: {{ .key }}
clusters:
- name: {{ .cluster }}
  cluster:
    certificate-authority-data: {{ .ca }}
    server: {{ .master }}
contexts:
- context:
    cluster: {{ .cluster }}
    user: {{ .username }}
  name: default
current-context: default
preferences: {}
`
)

// GenerateKubeconfig generates kubeconfig for given user
func GenerateKubeconfig(user, clusterName, apiserverDomain string, groups []string, rootCA *vcpki.CrtKeyPair) (string, error) {
	caPair, err := vcpki.NewClientCrtAndKey(user, rootCA, groups)
	if err != nil {
		return "", err
	}
	return generateKubeconfigUseCertAndKey(clusterName,
		[]string{apiserverDomain}, rootCA.Crt, caPair, user)
}

// encodeCertPEM encodes x509 certificate to pem
func encodeCertPEM(cert *x509.Certificate) []byte {
	block := pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	return pem.EncodeToMemory(&block)
}

// encodePrivateKeyPEM encodes rsa key to pem
func encodePrivateKeyPEM(private *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Bytes: x509.MarshalPKCS1PrivateKey(private),
		Type:  "RSA PRIVATE KEY",
	})
}

// generateKubeconfigUseCertAndKey generates kubeconfig based on the given crt/key pair
func generateKubeconfigUseCertAndKey(clusterName string, ips []string, apiserverCA *x509.Certificate, caPair *vcpki.CrtKeyPair, username string) (string, error) {
	urls := make([]string, 0, len(ips))
	for _, ip := range ips {
		urls = append(urls, fmt.Sprintf("https://%v:6443", ip))
	}
	ctx := map[string]string{
		"ca":       base64.StdEncoding.EncodeToString(encodeCertPEM(apiserverCA)),
		"key":      base64.StdEncoding.EncodeToString(encodePrivateKeyPEM(caPair.Key)),
		"cert":     base64.StdEncoding.EncodeToString(encodeCertPEM(caPair.Crt)),
		"username": username,
		"master":   strings.Join(urls, ","),
		"cluster":  clusterName,
	}

	return getTemplateContent(kubeconfigTemplate, ctx)
}

// getTemplateContent fills out the kubeconfig templates based on the context
func getTemplateContent(kubeConfigTmpl string, context interface{}) (string, error) {
	t, tmplPrsErr := template.New("test").Parse(kubeConfigTmpl)
	if tmplPrsErr != nil {
		return "", tmplPrsErr
	}
	writer := bytes.NewBuffer([]byte{})
	if err := t.Execute(writer, context); nil != err {
		return "", err
	}

	return writer.String(), nil
}
