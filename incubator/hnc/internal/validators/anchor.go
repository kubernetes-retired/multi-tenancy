package validators

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	k8sadm "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/config"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
)

// AnchorServingPath is where the validator will run. Must be kept in sync with the
// kubebuilder markers below.
const (
	AnchorServingPath = "/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchors"
)

// Note: the validating webhook FAILS CLOSE. This means that if the webhook goes down, all further
// changes are forbidden.
//
// +kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha2-subnamespaceanchors,mutating=false,failurePolicy=fail,groups="hnc.x-k8s.io",resources=subnamespaceanchors,verbs=create;delete,versions=v1alpha2,name=subnamespaceanchors.hnc.x-k8s.io

type Anchor struct {
	Log     logr.Logger
	Forest  *forest.Forest
	decoder *admission.Decoder
}

// req defines the aspects of the admission.Request that we care about.
type anchorRequest struct {
	anchor *api.SubnamespaceAnchor
	op     k8sadm.Operation
}

// Handle implements the validation webhook.
func (v *Anchor) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := v.Log.WithValues("ns", req.Namespace, "nm", req.Name, "op", req.Operation, "user", req.UserInfo.Username)
	// Early exit since the HNC SA can do whatever it wants.
	if isHNCServiceAccount(&req.AdmissionRequest.UserInfo) {
		log.V(1).Info("Allowed change by HNC SA")
		return allow("HNC SA")
	}

	decoded, err := v.decodeRequest(log, req)
	if err != nil {
		log.Error(err, "Couldn't decode request")
		return deny(metav1.StatusReasonBadRequest, err.Error())
	}
	if decoded == nil {
		// https://github.com/kubernetes-sigs/multi-tenancy/issues/688
		return allow("")
	}

	resp := v.handle(decoded)
	if !resp.Allowed {
		log.Info("Denied", "code", resp.Result.Code, "reason", resp.Result.Reason, "message", resp.Result.Message)
	} else {
		log.V(1).Info("Allowed", "message", resp.Result.Message)
	}
	return resp
}

// handle implements the non-boilerplate logic of this validator, allowing it to be more easily unit
// tested (ie without constructing a full admission.Request). It validates that the request is allowed
// based on the current in-memory state of the forest.
func (v *Anchor) handle(req *anchorRequest) admission.Response {
	v.Forest.Lock()
	defer v.Forest.Unlock()

	pnm := req.anchor.Namespace
	cnm := req.anchor.Name
	cns := v.Forest.Get(cnm)

	switch req.op {
	case k8sadm.Create:
		// Can't create subnamespaces in excluded namespaces
		if config.EX[pnm] {
			msg := fmt.Sprintf("The namespace %s is not allowed to create subnamespaces. Please create subnamespaces in a different namespace.", pnm)
			return deny(metav1.StatusReasonForbidden, msg)
		}

		// Can't create anchors for existing namespaces, _unless_ it's for a subns with a missing
		// anchor.
		if cns.Exists() {
			childIsMissingAnchor := (cns.Parent().Name() == pnm && cns.IsSub)
			if !childIsMissingAnchor {
				msg := fmt.Sprintf("The requested namespace %s already exists. Please use a different name.", cnm)
				return deny(metav1.StatusReasonConflict, msg)
			}
		}

	case k8sadm.Delete:
		// Don't allow the anchor to be deleted if it's in a good state and has descendants of its own,
		// unless allowCascadingDeletion is set.
		if req.anchor.Status.State == api.Ok && cns.ChildNames() != nil && !cns.AllowsCascadingDeletion() {
			msg := fmt.Sprintf("The subnamespace %s is not a leaf and doesn't allow cascading deletion. Please set allowCascadingDeletion flag or make it a leaf first.", cnm)
			return deny(metav1.StatusReasonForbidden, msg)
		}

	default:
		// nop for updates etc
	}

	return allow("")
}

// decodeRequest gets the information we care about into a simple struct that's easy to both a) use
// and b) factor out in unit tests.
func (v *Anchor) decodeRequest(log logr.Logger, in admission.Request) (*anchorRequest, error) {
	anchor := &api.SubnamespaceAnchor{}
	var err error
	// For DELETE request, use DecodeRaw() from req.OldObject, since Decode() only uses req.Object,
	// which will be empty for a DELETE request.
	if in.Operation == k8sadm.Delete {
		log.V(1).Info("Decoding a delete request.")
		if in.OldObject.Raw == nil {
			// https://github.com/kubernetes-sigs/multi-tenancy/issues/688
			return nil, nil
		}
		err = v.decoder.DecodeRaw(in.OldObject, anchor)
	} else {
		err = v.decoder.Decode(in, anchor)
	}
	if err != nil {
		return nil, err
	}

	return &anchorRequest{
		anchor: anchor,
		op:     in.Operation,
	}, nil
}

func (v *Anchor) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
