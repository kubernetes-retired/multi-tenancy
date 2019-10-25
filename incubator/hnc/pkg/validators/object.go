package validators

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// ObjectsServingPath is where the validator will run. Must be kept in sync with the
// kubebuilder markers below.
const (
	ObjectsServingPath = "/validate-objects"
)

// Note: the validating webhook FAILS OPEN. This means that if the webhook goes down, all further
// changes to the objects are allowed.
//
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="",resources=secrets,verbs=create;update,versions=v1,name=secrets
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="rbac.authorization.k8s.io",resources=rols,verbs=create;update,versions=v1,name=roles.rbac.authorization.k8s.io
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="rbac.authorization.k8s.io",resources=rolebindings,verbs=create;update,versions=v1,name=rolebindings.rbac.authorization.k8s.io
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="networking.k8s.io",resources=networkpolicies,verbs=create;update,versions=v1,name=networkpolicies.networking.k8s.io
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="",resources=resourcequotas,verbs=create;update,versions=v1,name=resourcesquotas
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="",resources=limitranges,verbs=create;update,versions=v1,name=limitranges
// +kubebuilder:webhook:path=/validate-objects,mutating=false,failurePolicy=ignore,groups="",resources=configmaps,verbs=create;update,versions=v1,name=configmaps

type Object struct {
	Log     logr.Logger
	Forest  *forest.Forest
	client  client.Client
	decoder *admission.Decoder
}

func (o *Object) Handle(ctx context.Context, req admission.Request) admission.Response {
	log := o.Log.WithValues("nm", req.Name, "nnm", req.Namespace)
	if isHNCServiceAccount(&req.AdmissionRequest.UserInfo) {
		log.Info("Allowed change by HNC SA")
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
	log.Info("Handled", "allowed", resp.Allowed, "code", resp.Result.Code, "reason", resp.Result.Reason, "message", resp.Result.Message)
	return resp
}

// handle implements the non-webhook-y businesss logic of this validator, allowing it to be more
// easily unit tested (ie without constructing an admission.Request, setting up user infos, etc).
func (o *Object) handle(ctx context.Context, log logr.Logger, inst *unstructured.Unstructured, oldInst *unstructured.Unstructured) admission.Response {
	oldValue, oldExists := getLabel(oldInst, api.LabelInheritedFrom)
	newValue, newExists := getLabel(inst, api.LabelInheritedFrom)
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

type apiInstances interface {
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
}

func setLabel(inst apiInstances, label string, value string) {
	labels := inst.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[label] = value
	inst.SetLabels(labels)
}

func getLabel(inst apiInstances, label string) (string, bool) {
	labels := inst.GetLabels()
	if labels == nil {
		return "", false
	}
	value, ok := labels[label]
	return value, ok
}
