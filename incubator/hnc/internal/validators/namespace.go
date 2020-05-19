package validators

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
)

// NamespaceServingPath is where the validator will run. Must be kept in sync with the
// kubebuilder markers below.
const (
	NamespaceServingPath = "/validate-v1-namespace"
)

// Note: the validating webhook FAILS CLOSE. This means that if the webhook goes down, all further
// changes are forbidden.
//
// +kubebuilder:webhook:path=/validate-v1-namespace,mutating=false,failurePolicy=fail,groups="",resources=namespaces,verbs=delete,versions=v1,name=namespaces.hnc.x-k8s.io

type Namespace struct {
	Log     logr.Logger
	Forest  *forest.Forest
	decoder *admission.Decoder
}

// nsRequest defines the aspects of the admission.Request that we care about.
type nsRequest struct {
	ns *corev1.Namespace
	op v1beta1.Operation
}

// Handle implements the validation webhook.
func (v *Namespace) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := v.Log.WithValues("NamespaceName", req.Name)
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

	resp := v.handle(decoded)
	log.V(1).Info("Handled", "allowed", resp.Allowed, "code", resp.Result.Code, "reason", resp.Result.Reason, "message", resp.Result.Message)
	return resp
}

// handle implements the non-boilerplate logic of this validator, allowing it to be more easily unit
// tested (ie without constructing a full admission.Request).
func (v *Namespace) handle(req *nsRequest) admission.Response {
	parent := req.ns.Annotations[api.SubnamespaceOf]
	if parent != "" && req.op == v1beta1.Delete {
		msg := fmt.Sprintf("The namespace %s is a subnamespace. Please delete the anchor from the parent namespace %s to delete the subnamespace.", req.ns.Name, parent)
		return deny(metav1.StatusReasonForbidden, msg)
	}

	return v.checkForest(req)
}

func (v *Namespace) checkForest(req *nsRequest) admission.Response {
	v.Forest.Lock()
	defer v.Forest.Unlock()

	ns := v.Forest.Get(req.ns.Name)

	// Early exit to allow non-delete requests or if the namespace allows cascading deletion.
	if req.op != v1beta1.Delete || ns.AllowsCascadingDelete() {
		return allow("")
	}

	// Check if the deleting namespace has subnamespaces that can't be deleted.
	cantDelete := []string{}
	for _, cnm := range ns.ChildNames() {
		cns := v.Forest.Get(cnm)
		if cns.IsSub && !cns.AllowsCascadingDelete() {
			cantDelete = append(cantDelete, cnm)
		}
	}
	if len(cantDelete) != 0 {
		msg := fmt.Sprintf("Please set allowCascadingDelete first either in the parent namespace or in all the subnamespaces.\n  Subnamespace(s) without allowCascadingDelete set: %s.", cantDelete)
		return deny(metav1.StatusReasonForbidden, msg)
	}
	return allow("")
}

// decodeRequest gets the information we care about into a simple struct that's easy to both a) use
// and b) factor out in unit tests.
func (v *Namespace) decodeRequest(log logr.Logger, in admission.Request) (*nsRequest, error) {
	ns := &corev1.Namespace{}
	var err error
	// For DELETE request, use DecodeRaw() from req.OldObject, since Decode() only uses req.Object,
	// which will be empty for a DELETE request.
	if in.Operation == v1beta1.Delete {
		log.V(1).Info("Decoding a delete request.")
		err = v.decoder.DecodeRaw(in.OldObject, ns)
	} else {
		err = v.decoder.Decode(in, ns)
	}
	if err != nil {
		return nil, err
	}

	return &nsRequest{
		ns: ns,
		op: in.Operation,
	}, nil
}

func (v *Namespace) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}
