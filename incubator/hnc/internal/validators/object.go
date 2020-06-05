package validators

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
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
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="*",resources="*",verbs=create;update,versions="*",name=objects.hnc.x-k8s.io

type Object struct {
	Log     logr.Logger
	Forest  *forest.Forest
	client  client.Client
	decoder *admission.Decoder
}

func (o *Object) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := o.Log.WithValues("nm", req.Name, "nnm", req.Namespace)
	// Allow changes to the types that are not in propagate mode. This is to dynamically enable/disable
	// object webhooks based on the types configured in hncconfig. Since the current admission rules only
	// apply to propagated objects, we can disable object webhooks on all other non-propagate-mode types.
	if !o.isPropagateType(req.Kind) {
		return allow("Non-propagate-mode types")
	}
	if isHNCServiceAccount(&req.AdmissionRequest.UserInfo) {
		log.V(1).Info("Allowed change by HNC SA")
		return allow("HNC SA")
	}

	inst := &unstructured.Unstructured{}
	if err := o.decoder.Decode(req, inst); err != nil {
		log.Error(err, "Couldn't decode req.Object", "raw", req.Object)
		return deny(metav1.StatusReasonBadRequest, err.Error())
	}
	log = log.WithValues("object", inst.GetName())

	oldInst := &unstructured.Unstructured{}
	// req.OldObject is the existing object. DecodeRaw will return an error if it's empty, so we should skip the decoding here.
	if len(req.OldObject.Raw) > 0 {
		if err := o.decoder.DecodeRaw(req.OldObject, oldInst); err != nil {
			log.Error(err, "Couldn't decode req.OldObject", "raw", req.OldObject)
			return deny(metav1.StatusReasonBadRequest, err.Error())
		}
	}

	resp := o.handle(ctx, log, inst, oldInst)
	log.V(1).Info("Handled", "allowed", resp.Allowed, "code", resp.Result.Code, "reason", resp.Result.Reason, "message", resp.Result.Message)
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
func (o *Object) handle(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured, oldInst *unstructured.Unstructured) admission.Response {
	// We want to ignore validation for objects in the exclusion list.
	if config.EX[inst.GetNamespace()] {
		return allow("")
	}

	// Prevent users from changing the InheritedFrom label
	oldValue, oldExists := metadata.GetLabel(oldInst, api.LabelInheritedFrom)
	newValue, newExists := metadata.GetLabel(inst, api.LabelInheritedFrom)

	// If old object holds the label but the new one doesn't, reject it. Vice versa.
	if oldExists != newExists {
		verb := "add"
		if !newExists {
			verb = "remove"
		}
		return deny(metav1.StatusReasonForbidden, "Users should not "+verb+" the label "+api.LabelInheritedFrom)
	}
	// If both old and new objects hold the label but with different values, reject it.
	if newExists && newValue != oldValue {
		return deny(metav1.StatusReasonForbidden, "Users should not change the value of label "+api.LabelInheritedFrom)
	}

	// If the existing object has an inheritedFrom label, it's a propagated object.
	// Any user changes should be rejected.
	if newExists && !reflect.DeepEqual(object.Canonical(inst), object.Canonical(oldInst)) {
		return deny(metav1.StatusReasonForbidden,
			"Illegal modification to an object propagated by the Hierarchical Namespace Controller")
	}

	return allow("")
}

func (o *Object) InjectClient(c client.Client) error {
	o.client = c
	return nil
}

func (o *Object) InjectDecoder(d *admission.Decoder) error {
	o.decoder = d
	return nil
}
