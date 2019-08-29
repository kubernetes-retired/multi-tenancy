package validators

import (
	"context"
	"net/http"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

const (
	// HierarchyServingPath is where the validator will run. Must be kept in sync with the
	// kubebuilder marker below.
	HierarchyServingPath = "/validate-hnc-x-k8s-io-v1alpha1-hierarchies"
)

// +kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha1-hierarchies,mutating=false,failurePolicy=fail,groups="hnc.x-k8s.io",resources=hierarchies,verbs=create;update,versions=v1alpha1,name=hierarchies.hnc.x-k8s.io

type Hierarchy struct {
	Log     logr.Logger
	client  client.Client
	decoder *admission.Decoder
}

func (v *Hierarchy) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := v.Log.WithValues("namespace", req.Namespace)
	log.Info("Validating")
	hier := &tenancy.Hierarchy{}
	err := v.decoder.Decode(req, hier)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}
	log.Info("Checking", "contents", hier)
	return admission.Allowed("hey ho")
}

func (v *Hierarchy) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

func (v *Hierarchy) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
