/*
Copyright 2020 The Kubernetes Authors.

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

package virtualcluster

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	cryptorand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	admv1beta1 "k8s.io/api/admissionregistration/v1beta1"
	certapi "k8s.io/api/certificates/v1beta1"
	certificates "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	certificatesclient "k8s.io/client-go/kubernetes/typed/certificates/v1beta1"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/certificate/csr"
	"k8s.io/client-go/util/keyutil"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	tenancyv1alpha1 "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/constants"
	kubeutil "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/controller/util/kube"
)

const (
	VCWebhookCertCommonName   = "virtualcluster-webhook"
	VCWebhookCertOrg          = "virtualcluster"
	VCWebhookCertFileName     = "tls.crt"
	VCWebhookKeyFileName      = "tls.key"
	VCWebhookServiceName      = "virtualcluster-webhook-service"
	DefaultVCWebhookServiceNs = "vc-manager"
	VCWebhookCfgName          = "virtualcluster-validating-webhook-configuration"
	VCWebhookCSRName          = "virtualcluster-webhook-csr"
)

var (
	VCWebhookServiceNs string
	log                = logf.Log.WithName("virtualcluster-webhook")
)

func init() {
	VCWebhookServiceNs, _ = kubeutil.GetPodNsFromInside()
	if VCWebhookServiceNs == "" {
		log.Info("setup virtualcluster webhook in default namespace",
			"default-ns", DefaultVCWebhookServiceNs)
		VCWebhookServiceNs = DefaultVCWebhookServiceNs
	}
}

// Add adds the webhook server to the manager as a runnable
func Add(mgr manager.Manager, certDir string) error {
	// 1. create the webhook service
	if err := createVirtualClusterWebhookService(mgr.GetClient()); err != nil {
		return fmt.Errorf("fail to create virtualcluster webhook service: %s", err)
	}

	// 2. generate the serving certificate for the webhook server
	if err := genCertificate(mgr, certDir); err != nil {
		return fmt.Errorf("fail to generate certificates for webhook server: %s", err)
	}

	// 3. create the ValidatingWebhookConfiguration
	log.Info(fmt.Sprintf("will create validatingwebhookconfiguration/%s", VCWebhookCfgName))
	if err := createValidatingWebhookConfiguration(mgr.GetClient()); err != nil {
		return fmt.Errorf("fail to create validating webhook configuration: %s", err)
	}
	log.Info(fmt.Sprintf("successfully created validatingwebhookconfiguration/%s", VCWebhookCfgName))

	// 4. register the validating webhook
	return (&tenancyv1alpha1.VirtualCluster{}).SetupWebhookWithManager(mgr)
}

// createVirtualClusterWebhookService creates the service for exposing the webhook server
func createVirtualClusterWebhookService(client client.Client) error {
	whSvc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      VCWebhookServiceName,
			Namespace: VCWebhookServiceNs,
			Labels: map[string]string{
				"virtualcluster-webhook": "true",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Port:       constants.VirtualClusterWebhookPort,
					TargetPort: intstr.FromInt(constants.VirtualClusterWebhookPort),
				},
			},
			Selector: map[string]string{
				"virtualcluster-webhook": "true",
			},
		},
	}
	if err := client.Create(context.TODO(), &whSvc); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		log.Info(fmt.Sprintf("service/%s already exist", VCWebhookServiceName))
		return nil
	}
	log.Info(fmt.Sprintf("successfully created service/%s", VCWebhookServiceName))
	return nil
}

// createValidatingWebhookConfiguration creates the validatingwebhookconfiguration for the webhook
func createValidatingWebhookConfiguration(client client.Client) error {
	validatePath := "/validate-tenancy-x-k8s-io-v1alpha1-virtualcluster"
	svcPort := int32(constants.VirtualClusterWebhookPort)
	// reject request if the webhook doesn't work
	failPolicy := admv1beta1.Fail
	vwhCfg := admv1beta1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: VCWebhookCfgName,
			Labels: map[string]string{
				"virtualcluster-webhook": "true",
			},
		},
		Webhooks: []admv1beta1.ValidatingWebhook{
			{
				Name: "virtualcluster.validating.webhook",
				ClientConfig: admv1beta1.WebhookClientConfig{
					Service: &admv1beta1.ServiceReference{
						Name:      VCWebhookServiceName,
						Namespace: VCWebhookServiceNs,
						Path:      &validatePath,
						Port:      &svcPort,
					},
				},
				FailurePolicy: &failPolicy,
				Rules: []admv1beta1.RuleWithOperations{
					{
						Operations: []admv1beta1.OperationType{
							admv1beta1.OperationAll,
						},
						Rule: admv1beta1.Rule{
							APIGroups:   []string{"tenancy.x-k8s.io"},
							APIVersions: []string{"v1alpha1"},
							Resources:   []string{"virtualclusters"},
						},
					},
				},
			},
		},
	}

	if err := client.Create(context.TODO(), &vwhCfg); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		log.Info(fmt.Sprintf("validatingwebhookconfiguration/%s already exist", VCWebhookCfgName))
		return nil
	}
	log.Info(fmt.Sprintf("successfully created validatingwebhookconfiguration/%s", VCWebhookCfgName))
	return nil
}

// genCertificate generates the serving cerficiate for the webhook server
func genCertificate(mgr manager.Manager, certDir string) error {
	// client-go client for generating certificate
	clientSet, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("fail to generate the clientset: %s", err)
	}
	csrClient := clientSet.CertificatesV1beta1().CertificateSigningRequests()

	// 1. delete the VC CSR if exist
	if err := delVCWebhookCSRIfExist(csrClient); err != nil {
		return fmt.Errorf("fail to delete existing webhook: %s", err)
	}
	// 2. generate csr
	csrPEM, keyPEM, privateKey, err := generateCSR(clientSet)
	if err != nil {
		return fmt.Errorf("fail to geneate csr: %s", err)
	}

	// 3. approve the csr
	go approveVCWebhookCSR(csrClient)

	// 4. submit csr and wait for it to be signed
	// NOTE this step will block until the CSR is issued
	csrPEM, err = submitCSRAndWait(csrClient, csrPEM, privateKey)
	if err != nil {
		return fmt.Errorf("fail to submit CSR: %s", err)
	}

	// 5. generate certificate files (i.e., tls.crt and tls.key)
	if err := genCertAndKeyFile(csrPEM, keyPEM, certDir); err != nil {
		return fmt.Errorf("fail to generate certificate and key: %s", err)
	}

	return nil
}

// delVCWebhookCSRIfExist deletes the validatingwebhookconfiguration/<VCWebhookCfgName> if exist
func delVCWebhookCSRIfExist(csrClient certificatesclient.CertificateSigningRequestInterface) error {
	if err := csrClient.Delete(context.TODO(), VCWebhookCSRName, metav1.DeleteOptions{}); err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		log.Info("there is no legacy virtualcluster-webhook CSR")
	}
	return nil
}

// approveVCWebhookCSR approves the first observered CSR whose name is <VCWebhookCSRName>,
// CommonName is <VCWebhookCertCommonName>, and Organization is <VCWebhookCertOrg>
func approveVCWebhookCSR(csrClient certificatesclient.CertificateSigningRequestInterface) {
	_, _ = watchtools.ListWatchUntil(
		context.Background(),
		&cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return csrClient.List(context.TODO(), options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return csrClient.Watch(context.TODO(), options)
			},
		},
		func(event watch.Event) (bool, error) {
			switch event.Type {
			// only react to Modified and Added event
			case watch.Modified, watch.Added:
			case watch.Deleted:
				return false, nil
			default:
				return false, nil
			}
			csr := event.Object.(*certificates.CertificateSigningRequest)
			if !isVCWebhookCSR(csr) {
				return false, nil
			}

			approved, denied := getCertCondition(&csr.Status)
			// return if the CSR has already been approved or denied
			if approved || denied {
				return true, nil
			}

			log.Info("will try to approve the CSR", "CSR", csr.GetName())
			// approve the virtualcluster webhook csr
			csr.Status.Conditions = append(csr.Status.Conditions,
				certapi.CertificateSigningRequestCondition{
					Type:    certapi.CertificateApproved,
					Reason:  "AutoApproved",
					Message: fmt.Sprintf("Approve the csr/%s", csr.GetName()),
				})

			result, err := csrClient.UpdateApproval(context.TODO(), csr, metav1.UpdateOptions{})
			if err != nil {
				if result == nil {
					log.Error(err, fmt.Sprintf("failed to approve virtualcluster csr, %v", err))
				} else {
					log.Error(err, fmt.Sprintf("failed to approve virtualcluster csr(%s), %v", result.Name, err))
				}
				return false, err
			}
			log.Info("successfully approve virtualcluster csr", "csr", result.Name)
			return true, nil
		},
	)
}

// isVCWebhookCSR check if the given csr is a VirtualCluster Webhook related csr
func isVCWebhookCSR(csr *certificates.CertificateSigningRequest) bool {
	pemBytes := csr.Spec.Request
	block, _ := pem.Decode(pemBytes)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return false
	}
	x509cr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return false
	}

	if x509cr.Subject.CommonName != VCWebhookCertCommonName {
		return false
	}

	if csr.GetName() != VCWebhookCSRName {
		log.Info(fmt.Sprintf("will not approve CSR, only approve CSR with name %s", VCWebhookCSRName),
			"CSR", csr.GetName())
		return false
	}

	for _, org := range x509cr.Subject.Organization {
		if org == VCWebhookCertOrg {
			return true
		}
	}
	return false
}

// getCertCondition checks if the given CSR status is approved or denied
func getCertCondition(status *certificates.CertificateSigningRequestStatus) (bool, bool) {
	var approved, denied bool
	for _, c := range status.Conditions {
		if c.Type == certificates.CertificateApproved {
			approved = true
		}
		if c.Type == certificates.CertificateDenied {
			denied = true
		}
	}
	return approved, denied
}

// getSVCClusterIP returns the clusterIP of the webhook service, which will be written
// into the certificate
func getSVCClusterIP(clientSet kubernetes.Interface) (net.IP, error) {
	whSvc, err := clientSet.CoreV1().Services(VCWebhookServiceNs).
		Get(context.TODO(), VCWebhookServiceName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("fail to get serivce clusterIP: %s", err)
	}
	if whSvc.Spec.ClusterIP == "" {
		return nil, fmt.Errorf("ClusterIP of the service/%s is not set", VCWebhookServiceName)
	}
	return net.ParseIP(whSvc.Spec.ClusterIP), nil
}

// genCertAndKeyFile creates the serving certificate/key files for the webhook server
func genCertAndKeyFile(certData, keyData []byte, certDir string) error {
	// always remove first
	if err := os.RemoveAll(certDir); err != nil {
		return fmt.Errorf("fail to remove certificates: %s", err)
	}
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("could not create directory %q to store certificates: %v", certDir, err)
	}
	certPath := filepath.Join(certDir, VCWebhookCertFileName)
	f, err := os.OpenFile(certPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("could not open %q: %v", certPath, err)
	}
	defer f.Close()
	certBlock, _ := pem.Decode(certData)
	if certBlock == nil {
		return fmt.Errorf("invalid certificate data")
	}
	pem.Encode(f, certBlock)

	keyPath := filepath.Join(certDir, VCWebhookKeyFileName)
	kf, err := os.OpenFile(keyPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("could not open %q: %v", keyPath, err)
	}

	keyBlock, _ := pem.Decode(keyData)
	if keyBlock == nil {
		return fmt.Errorf("invalid key data")
	}
	pem.Encode(kf, keyBlock)
	log.Info("successfully generate certificate and key file")
	return nil
}

// submitCSRAndWait submits the CSR and wait for apiserver to signed it
func submitCSRAndWait(csrClient certificatesclient.CertificateSigningRequestInterface,
	csrPEM []byte,
	privateKey interface{}) ([]byte, error) {
	req, err := csr.RequestCertificate(
		csrClient, csrPEM, VCWebhookCSRName, "kubernetes.io/legacy-unknown",
		[]certificates.KeyUsage{certificates.UsageServerAuth},
		privateKey)
	if err != nil {
		return nil, fmt.Errorf("fail to request certificate: %s", err)
	}
	log.Info("CSR request submitted, will wait 2 seconds for it to be signed", "CSR reqest", req.GetName())
	timeoutCtx, cancelFn := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelFn()
	crtPEM, err := csr.WaitForCertificate(timeoutCtx, csrClient, req)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Certificate request was not signed: %v", err))
		return nil, nil
	}
	log.Info("CSR is signed", "CSR reqest", req.GetName())
	return crtPEM, nil
}

// generateCSR generate the csrPEM and corresponding keyPEM
func generateCSR(clientSet kubernetes.Interface) (csrPEM []byte, keyPEM []byte, key interface{}, err error) {
	// Generate a new private key.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), cryptorand.Reader)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to generate a new private key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to marshal the new key to DER: %v", err)
	}

	keyPEM = pem.EncodeToMemory(&pem.Block{Type: keyutil.ECPrivateKeyBlockType, Bytes: der})

	whSvcIP, err := getSVCClusterIP(clientSet)
	if err != nil {
		return nil, nil, nil, err
	}

	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   VCWebhookCertCommonName,
			Organization: []string{VCWebhookCertOrg},
		},
		DNSNames: []string{
			VCWebhookServiceName,
			VCWebhookServiceName + "." + VCWebhookServiceNs,
			VCWebhookServiceName + "." + VCWebhookServiceNs + ".svc",
		},
		IPAddresses: []net.IP{whSvcIP},
	}

	csrPEM, err = cert.MakeCSRFromTemplate(privateKey, template)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to create a csr from the private key: %v", err)
	}
	log.Info("CSR is generated")
	return csrPEM, keyPEM, privateKey, nil
}
