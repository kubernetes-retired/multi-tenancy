package validators

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	cert "github.com/open-policy-agent/cert-controller/pkg/rotator"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
)

const (
	serviceName     = "hnc-webhook-service"
	vwhName         = "hnc-validating-webhook-configuration"
	caName          = "hnc-ca"
	caOrganization  = "hnc"
	secretNamespace = "hnc-system"
	secretName      = "hnc-webhook-server-cert"
	certDir         = "/tmp/k8s-webhook-server/serving-certs"
)

var crds = []string{"hncconfigurations.hnc.x-k8s.io", "subnamespaceanchors.hnc.x-k8s.io", "hierarchyconfigurations.hnc.x-k8s.io"}

// DNSName is <service name>.<namespace>.svc
var dnsName = fmt.Sprintf("%s.%s.svc", serviceName, secretNamespace)

// CreateCertsIfNeeded creates all certs for webhooks. This function is called from main.go.
func CreateCertsIfNeeded(mgr ctrl.Manager, novalidation, internalCert bool) (chan struct{}, error) {
	setupFinished := make(chan struct{})
	if novalidation || !internalCert {
		close(setupFinished)
		return setupFinished, nil
	}

	return setupFinished, cert.AddRotator(mgr, &cert.CertRotator{
		SecretKey: types.NamespacedName{
			Namespace: secretNamespace,
			Name:      secretName,
		},
		CertDir:        certDir,
		CAName:         caName,
		CAOrganization: caOrganization,
		DNSName:        dnsName,
		IsReady:        setupFinished,
		VWHName:        vwhName,
		CRDNames:       crds,
	})
}

// Create creates all validators. This function is called from main.go.
func Create(mgr ctrl.Manager, f *forest.Forest) {
	// Create webhook for Hierarchy
	mgr.GetWebhookServer().Register(HierarchyServingPath, &webhook.Admission{Handler: &Hierarchy{
		Log:    ctrl.Log.WithName("validators").WithName("Hierarchy"),
		Forest: f,
	}})

	// Create webhooks for managed objects
	mgr.GetWebhookServer().Register(ObjectsServingPath, &webhook.Admission{Handler: &Object{
		Log:    ctrl.Log.WithName("validators").WithName("Object"),
		Forest: f,
	}})

	// Create webhook for the config
	mgr.GetWebhookServer().Register(ConfigServingPath, &webhook.Admission{Handler: &HNCConfig{
		Log:    ctrl.Log.WithName("validators").WithName("HNCConfig"),
		Forest: f,
	}})

	// Create webhook for the subnamespace anchors.
	mgr.GetWebhookServer().Register(AnchorServingPath, &webhook.Admission{Handler: &Anchor{
		Log:    ctrl.Log.WithName("validators").WithName("Anchor"),
		Forest: f,
	}})

	// Create webhook for the namespaces (core type).
	mgr.GetWebhookServer().Register(NamespaceServingPath, &webhook.Admission{Handler: &Namespace{
		Log:    ctrl.Log.WithName("validators").WithName("Namespace"),
		Forest: f,
	}})
}
