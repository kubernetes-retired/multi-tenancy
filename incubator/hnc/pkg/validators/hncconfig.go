package validators

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

// ConfigServingPath is where the validator will run. Must be kept in sync with the
// kubebuilder markers below.
const (
	ConfigServingPath = "/validate-hnc-x-k8s-io-v1alpha1-hncconfigurations"
)

// Note: the validating webhook FAILS CLOSE. This means that if the webhook goes down, all further
// changes are denied.
//
// +kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha1-hncconfigurations,mutating=false,failurePolicy=fail,groups="hnc.x-k8s.io",resources=hncconfigurations,verbs=create;update;delete,versions=v1alpha1,name=hncconfigurations.hnc.x-k8s.io

type HNCConfig struct {
	Log       logr.Logger
	validator gvkValidator
	decoder   *admission.Decoder
}

// gvkValidator checks if a resource exists. The check should typically be performed against the apiserver,
// but need to be stubbed out during unit testing.
type gvkValidator interface {
	// Exists takes a GVK and returns an error if the GVK does not exist in the apiserver.
	Exists(ctx context.Context, gvk schema.GroupVersionKind) error
}

type gvkSet map[schema.GroupVersionKind]bool

func (c *HNCConfig) Handle(ctx context.Context, req admission.Request) admission.Response {
	if isHNCServiceAccount(&req.AdmissionRequest.UserInfo) {
		return allow("HNC SA")
	}

	if req.Operation == v1beta1.Delete {
		if req.Name == "config" {
			return deny(metav1.StatusReasonForbidden, "Deleting the 'config' object is forbidden")
		} else {
			// We allow deleting other objects. If the validation controller has always been running, we should not
			// enter this case because objects of other names cannot be created. If users somehow bypass the
			// validation controller and create objects of other names, we will allow them to delete the objects.
			return allow("")
		}
	}

	inst := &api.HNCConfiguration{}
	if err := c.decoder.Decode(req, inst); err != nil {
		c.Log.Error(err, "Couldn't decode request")
		return deny(metav1.StatusReasonBadRequest, err.Error())
	}

	resp := c.handle(ctx, inst)
	c.Log.Info("Handled", "allowed", resp.Allowed, "code", resp.Result.Code, "reason", resp.Result.Reason, "message", resp.Result.Message)
	return resp
}

// handle implements the validation logic of this validator for Create and Update operations,
// allowing it to be more easily unit tested (ie without constructing a full admission.Request).
func (c *HNCConfig) handle(ctx context.Context, inst *api.HNCConfiguration) admission.Response {
	if inst.GetName() != "config" {
		return deny(metav1.StatusReasonInvalid, fmt.Sprintf("Wrong singleton name: %s; the name should be 'config'", inst.GetName()))
	}

	roleExist := false
	roleBindingExist := false
	ts := gvkSet{}
	for _, t := range inst.Spec.Types {
		if rp := c.isTypeConfigured(t, ts); !rp.Allowed {
			return rp
		}

		if c.isRBAC(t, "Role") {
			if rp := c.validateRBAC(t.Mode, "Role"); !rp.Allowed {
				return rp
			}
			roleExist = true
		} else if c.isRBAC(t, "RoleBinding") {
			if rp := c.validateRBAC(t.Mode, "RoleBinding"); !rp.Allowed {
				return rp
			}
			roleBindingExist = true
		} else {
			if rp := c.validateType(ctx, t, ts); !rp.Allowed {
				return rp
			}
		}
	}
	if !roleExist {
		return deny(metav1.StatusReasonInvalid, "Configuration for Role is missing")
	}
	if !roleBindingExist {
		return deny(metav1.StatusReasonInvalid, "Configuration for RoleBinding is missing")
	}
	return allow("")
}

// Validate if the configuration of a type already exists. Each type should only have one configuration.
func (c *HNCConfig) isTypeConfigured(t api.TypeSynchronizationSpec, ts gvkSet) admission.Response {
	gvk := schema.FromAPIVersionAndKind(t.APIVersion, t.Kind)
	if exists := ts[gvk]; exists {
		return deny(metav1.StatusReasonInvalid, fmt.Sprintf("Duplicate configurations for %s", gvk))
	}
	ts[gvk] = true
	return allow("")
}

func (c *HNCConfig) isRBAC(t api.TypeSynchronizationSpec, kind string) bool {
	return t.APIVersion == "rbac.authorization.k8s.io/v1" && t.Kind == kind
}

// The mode of Role and RoleBinding should be either unset or set to the propagate mode.
func (c *HNCConfig) validateRBAC(mode api.SynchronizationMode, kind string) admission.Response {
	if mode == api.Propagate || mode == "" {
		return allow("")
	}
	return deny(metav1.StatusReasonInvalid, fmt.Sprintf("Invalid mode of %s; current mode: %s; expected mode %s", kind, mode, api.Propagate))
}

// validateType validates a non-RBAC type.
func (c *HNCConfig) validateType(ctx context.Context, t api.TypeSynchronizationSpec, ts gvkSet) admission.Response {
	gvk := schema.FromAPIVersionAndKind(t.APIVersion, t.Kind)

	// Validate if the GVK exists in the apiserver.
	if err := c.validator.Exists(ctx, gvk); err != nil {
		return deny(metav1.StatusReasonInvalid,
			fmt.Sprintf("Cannot find the %s in the apiserver with error: %s", gvk, err.Error()))
	}

	// The mode of a type should be either unset or set to one of the supported modes.
	switch t.Mode {
	case api.Propagate, api.Ignore, api.Remove, "":
		return allow("")
	default:
		return deny(metav1.StatusReasonInvalid, fmt.Sprintf("Unrecognized mode '%s' for %s", t.Mode, gvk))
	}
}

// realGVKValidator implements gvkValidator, and is not used during unit tests.
type realGVKValidator struct {
	client client.Client
}

// Exists validates if a given GVK exists in the apiserver.
func (r *realGVKValidator) Exists(ctx context.Context, gvk schema.GroupVersionKind) error {
	inst := &unstructured.Unstructured{}
	inst.SetGroupVersionKind(gvk)
	err := r.client.Get(ctx, types.NamespacedName{Name: "nm"}, inst)
	// We try to get an object of the given GVK with name "nm". It is possible that the
	// object does not exist. Therefore, we will ignore the IsNotFound error.
	if errors.IsNotFound(err) {
		return nil
	}
	// if err is nil, that means the object was found, which means the type exists.
	return err
}

func (c *HNCConfig) InjectClient(cl client.Client) error {
	c.validator = &realGVKValidator{client: cl}
	return nil
}

func (c *HNCConfig) InjectDecoder(d *admission.Decoder) error {
	c.decoder = d
	return nil
}
