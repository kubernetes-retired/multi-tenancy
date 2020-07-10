package validators

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/config"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/metadata"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/object"
)

// ObjectsServingPath is where the validator will run. Must be kept in sync with the
// kubebuilder markers below.
const (
	ObjectsServingPath = "/validate-objects"
)

// Note: the validating webhook FAILS OPEN. This means that if the webhook goes down, all further
// changes to the objects are allowed.
//
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="*",resources="*",verbs=create;update;delete,versions="*",name=objects.hnc.x-k8s.io

type Object struct {
	Log     logr.Logger
	Forest  *forest.Forest
	client  client.Client
	decoder *admission.Decoder
}

func (o *Object) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := o.Log.WithValues("nm", req.Name, "resource", req.Resource, "nnm", req.Namespace, "op", req.Operation)

	// Before even looking at the objects, early-exit for any changes we shouldn't be involved in.
	// This reduces the chance we'll hose some aspect of the cluster we weren't supposed to touch.
	//
	// Firstly, skip namespaces we're excluded from (like kube-system).
	if config.EX[req.Namespace] {
		return allow("excluded namespace " + req.Namespace)
	}
	// Allow changes to the types that are not in propagate mode. This is to dynamically enable/disable
	// object webhooks based on the types configured in hncconfig. Since the current admission rules only
	// apply to propagated objects, we can disable object webhooks on all other non-propagate-mode types.
	if !o.isPropagateType(req.Kind) {
		return allow("Non-propagate-mode types")
	}
	// Finally, let the HNC SA do whatever it wants.
	if isHNCServiceAccount(&req.AdmissionRequest.UserInfo) {
		log.V(1).Info("Allowed change by HNC SA")
		return allow("HNC SA")
	}

	// Decode the old and new object, if we expect them to exist ("old" won't exist for creations,
	// while "new" won't exist for deletions).
	inst := &unstructured.Unstructured{}
	oldInst := &unstructured.Unstructured{}
	if req.Operation != admissionv1beta1.Delete {
		if err := o.decoder.Decode(req, inst); err != nil {
			log.Error(err, "Couldn't decode req.Object", "raw", req.Object)
			return deny(metav1.StatusReasonBadRequest, err.Error())
		}
	}
	if req.Operation != admissionv1beta1.Create {
		if err := o.decoder.DecodeRaw(req.OldObject, oldInst); err != nil {
			log.Error(err, "Couldn't decode req.OldObject", "raw", req.OldObject)
			return deny(metav1.StatusReasonBadRequest, err.Error())
		}
	}

	// Run the actual logic.
	resp := o.handle(ctx, log, req.Operation, inst, oldInst)

	level := 1
	if !resp.Allowed {
		level = 0
	}
	log.V(level).Info("Handled", "allowed", resp.Allowed, "code", resp.Result.Code, "reason", resp.Result.Reason, "message", resp.Result.Message)
	return resp
}

func (o *Object) isPropagateType(gvk metav1.GroupVersionKind) bool {
	o.Forest.Lock()
	defer o.Forest.Unlock()

	ts := o.Forest.GetTypeSyncerFromGroupKind(schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind})
	return ts != nil && ts.GetMode() == api.Propagate
}

// handle implements the non-webhook-y businesss logic of this validator, allowing it to be more
// easily unit tested (ie without constructing an admission.Request, setting up user infos, etc).
func (o *Object) handle(ctx context.Context, log logr.Logger, op admissionv1beta1.Operation, inst, oldInst *unstructured.Unstructured) admission.Response {
	// Find out if the object was/is inherited, and where it's inherited from.
	oldSource, oldInherited := metadata.GetLabel(oldInst, api.LabelInheritedFrom)
	newSource, newInherited := metadata.GetLabel(inst, api.LabelInheritedFrom)

	// If the object wasn't and isn't inherited, it's none of our business.
	if !oldInherited && !newInherited {
		return allow("source object")
	}

	// This is a propagated object. Propagated objects cannot be created or deleted (except by the HNC
	// SA, but the HNC SA never gets this far in the validation). They *can* have their statuses
	// updated, so if this is an update, make sure that the canonical form of the object hasn't
	// changed.
	switch op {
	case admissionv1beta1.Create:
		return deny(metav1.StatusReasonForbidden, "Cannot create objects with the label \""+api.LabelInheritedFrom+"\"")

	case admissionv1beta1.Delete:
		return deny(metav1.StatusReasonForbidden, "Cannot delete object propagated from namespace \""+oldSource+"\"")

	case admissionv1beta1.Update:
		// If the values have changed, that's an illegal modification. This includes if the label is
		// added or deleted. Note that this label is *not* included in object.Canonical(), below, so we
		// need to check it manually.
		if newSource != oldSource {
			return deny(metav1.StatusReasonForbidden, "Cannot modify the label \""+api.LabelInheritedFrom+"\"")
		}

		// If the existing object has an inheritedFrom label, it's a propagated object. Any user changes
		// should be rejected. Note that object.Canonical does *not* compare any HNC labels or
		// annotations.
		if !reflect.DeepEqual(object.Canonical(inst), object.Canonical(oldInst)) {
			return deny(metav1.StatusReasonForbidden,
				"Cannot modify object propagated from namespace \""+oldSource+"\"")
		}

		return allow("no illegal updates to propagated object")
	}

	// If you get here, it means the webhook config is misconfigured to include an operation that we
	// actually don't support.
	return deny(metav1.StatusReasonInternalError, "unknown operation: "+string(op))
}

func (o *Object) InjectClient(c client.Client) error {
	o.client = c
	return nil
}

func (o *Object) InjectDecoder(d *admission.Decoder) error {
	o.decoder = d
	return nil
}
