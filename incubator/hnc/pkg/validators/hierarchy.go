package validators

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/go-logr/logr"
	authenticationv1 "k8s.io/api/authentication/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	tenancy "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

const (
	// HierarchyServingPath is where the validator will run. Must be kept in sync with the
	// kubebuilder marker below.
	HierarchyServingPath = "/validate-hnc-x-k8s-io-v1alpha1-hierarchies"
)

// Note: the validating webhook FAILS CLOSED. This means that if the webhook goes down, all further
// changes to the hierarchy are forbidden. However, new objects will still be propagated according
// to the existing hierarchy (unless the reconciler is down too).
//
// +kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha1-hierarchies,mutating=false,failurePolicy=fail,groups="hnc.x-k8s.io",resources=hierarchies,verbs=create;update,versions=v1alpha1,name=hierarchies.hnc.x-k8s.io

type Hierarchy struct {
	Log     logr.Logger
	Forest  *forest.Forest
	client  client.Client
	decoder *admission.Decoder
}

func (v *Hierarchy) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := v.Log.WithValues("ns", req.Namespace)

	if isHNCServiceAccount(req.AdmissionRequest.UserInfo) {
		log.Info("Allowed change by HNC SA")
		return admission.Allowed("Change by HNC SA")
	}

	inst := &tenancy.Hierarchy{}
	err := v.decoder.Decode(req, inst)
	if err != nil {
		log.Error(err, "Couldn't decode request")
		return admission.Errored(http.StatusBadRequest, err)
	}

	return v.handle(ctx, log, inst)
}

// handle implements the non-webhook-y businesss logic of this validator, allowing it to be more
// easily unit tested (ie without constructing an admission.Request, setting up user infos, etc).
func (v *Hierarchy) handle(ctx context.Context, log logr.Logger, inst *tenancy.Hierarchy) admission.Response {
	nnm := inst.ObjectMeta.Namespace
	pnm := inst.Spec.Parent
	if pnm == "" {
		log.Info("Allowed", "parent", "<none>")
		return admission.Allowed("No parent set")
	}

	v.Forest.Lock()
	defer v.Forest.Unlock()
	ns := v.Forest.Get(nnm)
	pns := v.Forest.Get(pnm)
	if !pns.Exists() {
		// TODO: only allow if sufficient privileges
		log.Info("Allowed missing", "parent", pnm)
		return admission.Allowed("Parent does not exist yet")
	}
	if reason := ns.CanSetParent(pns); reason != "" {
		log.Info("Rejected", "parent", pnm, "reason", reason)
		return admission.Denied("Illegal parent: " + reason)
	}
	log.Info("Allowed", "parent", pnm)
	return admission.Allowed("Parent is legal")
}

// isHNCServiceAccount is inspired by isGKServiceAccount from open-policy-agent/gatekeeper.
func isHNCServiceAccount(user authenticationv1.UserInfo) bool {
	ns, found := os.LookupEnv("POD_NAMESPACE")
	if !found {
		ns = "hnc-system"
	}
	saGroup := fmt.Sprintf("system:serviceaccounts:%s", ns)
	for _, g := range user.Groups {
		if g == saGroup {
			return true
		}
	}
	return false
}

func (v *Hierarchy) InjectClient(c client.Client) error {
	v.client = c
	return nil
}

func (v *Hierarchy) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
