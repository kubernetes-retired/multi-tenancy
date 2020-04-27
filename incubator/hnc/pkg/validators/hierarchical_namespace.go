package validators

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// HierarchicalNamespaceServingPath is where the validator will run. Must be kept in sync with the
// kubebuilder markers below.
const (
	HierarchicalNamespaceServingPath = "/validate-hnc-x-k8s-io-v1alpha1-hierarchicalnamespaces"
)

// Note: the validating webhook FAILS CLOSE. This means that if the webhook goes down, all further
// changes are forbidden.
//
// +kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha1-hierarchicalnamespaces,mutating=false,failurePolicy=fail,groups="hnc.x-k8s.io",resources=hierarchicalnamespaces,verbs=create;delete,versions=v1alpha1,name=hierarchicalnamespaces.hnc.x-k8s.io

type HierarchicalNamespace struct {
	Log     logr.Logger
	Forest  *forest.Forest
	decoder *admission.Decoder
}

// req defines the aspects of the admission.Request that we care about.
type hnsRequest struct {
	hns *api.HierarchicalNamespace
	op  v1beta1.Operation
}

// Handle implements the validation webhook.
func (v *HierarchicalNamespace) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := v.Log.WithValues("Namespace", req.Namespace, "Name", req.Name)
	// Early exit since the HNC SA can do whatever it wants.
	if isHNCServiceAccount(&req.AdmissionRequest.UserInfo) {
		log.V(1).Info("Allowed change by HNC SA")
		return allow("HNC SA")
	}

	decoded, err := v.decodeRequest(log, req)
	if err != nil {
		v.Log.Error(err, "Couldn't decode request")
		return deny(metav1.StatusReasonBadRequest, err.Error())
	}

	resp := v.handle(decoded)
	log.V(1).Info("Handled", "allowed", resp.Allowed, "code", resp.Result.Code, "reason", resp.Result.Reason, "message", resp.Result.Message)
	return resp
}

// handle implements the non-boilerplate logic of this validator, allowing it to be more easily unit
// tested (ie without constructing a full admission.Request). It validates that the request is allowed
// based on the current in-memory state of the forest.
func (v *HierarchicalNamespace) handle(req *hnsRequest) admission.Response {
	v.Forest.Lock()
	defer v.Forest.Unlock()

	pnm := req.hns.Namespace
	cnm := req.hns.Name
	cns := v.Forest.Get(cnm)

	if req.op == v1beta1.Create {
		if config.EX[pnm] {
			msg := fmt.Sprintf("The namespace %s is not allowed to create subnamespaces. Please create subnamespaces in a different namespace.", pnm)
			return deny(metav1.StatusReasonForbidden, msg)
		}

		if cns.Exists() {
			msg := fmt.Sprintf("The requested namespace %s already exists. Please use a different name.", cnm)
			return deny(metav1.StatusReasonConflict, msg)
		}
	}

	if req.op == v1beta1.Delete {
		if cns.Exists() && !cns.AllowsCascadingDelete() {
			msg := fmt.Sprintf("The subnamespace %s doesn't allow cascading deletion. Please set allowCascadingDelete flag first.", cnm)
			return deny(metav1.StatusReasonForbidden, msg)
		}
	}

	return allow("")
}

// decodeRequest gets the information we care about into a simple struct that's easy to both a) use
// and b) factor out in unit tests.
func (v *HierarchicalNamespace) decodeRequest(log logr.Logger, in admission.Request) (*hnsRequest, error) {
	hns := &api.HierarchicalNamespace{}
	var err error
	// For DELETE request, use DecodeRaw() from req.OldObject, since Decode() only uses req.Object,
	// which will be empty for a DELETE request.
	if in.Operation == v1beta1.Delete {
		log.V(1).Info("Decoding a delete request.")
		err = v.decoder.DecodeRaw(in.OldObject, hns)
	} else {
		err = v.decoder.Decode(in, hns)
	}
	if err != nil {
		return nil, err
	}

	return &hnsRequest{
		hns: hns,
		op:  in.Operation,
	}, nil
}

func (v *HierarchicalNamespace) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
