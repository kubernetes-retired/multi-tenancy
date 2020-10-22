package validators

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
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
// +kubebuilder:webhook:path=/validate-v1-namespace,mutating=false,failurePolicy=fail,groups="",resources=namespaces,verbs=delete;create;update,versions=v1,name=namespaces.hnc.x-k8s.io

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
// tested (ie without constructing a full admission.Request).
func (v *Namespace) handle(req *nsRequest) admission.Response {
	v.Forest.Lock()
	defer v.Forest.Unlock()

	ns := v.Forest.Get(req.ns.Name)

	switch req.op {
	case v1beta1.Create:
		// This check only applies to the Create operation since namespace name
		// cannot be updated.
		if rsp := v.nameExistsInExternalHierarchy(req); !rsp.Allowed {
			return rsp
		}
	case v1beta1.Update:
		// This check only applies to the Update operation. Creating a namespace
		// with external manager is allowed and we will prevent this conflict by not
		// allowing setting a parent when validating the HierarchyConfiguration.
		if rsp := v.conflictBetweenParentAndExternalManager(req, ns); !rsp.Allowed {
			return rsp
		}
	case v1beta1.Delete:
		if rsp := v.cannotDeleteSubnamespace(req); !rsp.Allowed {
			return rsp
		}
		if rsp := v.illegalCascadingDeletion(ns); !rsp.Allowed {
			return rsp
		}
	}

	return allow("")
}

func (v *Namespace) nameExistsInExternalHierarchy(req *nsRequest) admission.Response {
	for _, nm := range v.Forest.GetNamespaceNames() {
		if _, ok := v.Forest.Get(nm).ExternalTreeLabels[req.ns.Name]; ok {
			msg := fmt.Sprintf("The namespace name %q is reserved by the external hierarchy manager %q.", req.ns.Name, v.Forest.Get(nm).Manager)
			return deny(metav1.StatusReasonAlreadyExists, msg)
		}
	}
	return allow("")
}

func (v *Namespace) conflictBetweenParentAndExternalManager(req *nsRequest, ns *forest.Namespace) admission.Response {
	mgr := req.ns.Annotations[api.AnnotationManagedBy]
	if mgr != "" && mgr != api.MetaGroup && ns.Parent() != nil {
		msg := fmt.Sprintf("Namespace %q is a child of %q. Namespaces with parents defined by HNC cannot also be managed externally. "+
			"To manage this namespace with %q, first make it a root in HNC.", ns.Name(), ns.Parent().Name(), mgr)
		return deny(metav1.StatusReasonForbidden, msg)
	}
	return allow("")
}

func (v *Namespace) cannotDeleteSubnamespace(req *nsRequest) admission.Response {
	parent := req.ns.Annotations[api.SubnamespaceOf]
	// Early exit if the namespace is not a subnamespace.
	if parent == "" {
		return allow("")
	}

	// If the anchor doesn't exist, we want to allow it to be deleted anyway.
	// See issue https://github.com/kubernetes-sigs/multi-tenancy/issues/847.
	anchorExists := v.Forest.Get(parent).HasAnchor(req.ns.Name)
	if anchorExists {
		msg := fmt.Sprintf("The namespace %s is a subnamespace. Please delete the anchor from the parent namespace %s to delete the subnamespace.", req.ns.Name, parent)
		return deny(metav1.StatusReasonForbidden, msg)
	}
	return allow("")
}

func (v *Namespace) illegalCascadingDeletion(ns *forest.Namespace) admission.Response {
	// Early exit if the namespace allows cascading deletion.
	if ns.AllowsCascadingDeletion() {
		return allow("")
	}

	// Check if all children allow cascading deletion.
	cantDelete := []string{}
	for _, cnm := range ns.ChildNames() {
		cns := v.Forest.Get(cnm)
		if cns.IsSub && !cns.AllowsCascadingDeletion() {
			cantDelete = append(cantDelete, cnm)
		}
	}
	if len(cantDelete) != 0 {
		msg := fmt.Sprintf("Please set allowCascadingDeletion first either in the parent namespace or in all the subnamespaces.\n  Subnamespace(s) without allowCascadingDelete set: %s.", cantDelete)
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
		if in.OldObject.Raw == nil {
			// See https://github.com/kubernetes-sigs/multi-tenancy/issues/688. OldObject can be nil in
			// K8s 1.14 and earlier.
			return nil, nil
		}
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
