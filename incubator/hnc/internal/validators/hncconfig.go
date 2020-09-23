package validators

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

// ConfigServingPath is where the validator will run. Must be kept in sync with the
// kubebuilder markers below.
const (
	ConfigServingPath = "/validate-hnc-x-k8s-io-v1alpha2-hncconfigurations"
)

// Note: the validating webhook FAILS CLOSE. This means that if the webhook goes down, all further
// changes are denied.
//
// +kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha2-hncconfigurations,mutating=false,failurePolicy=fail,groups="hnc.x-k8s.io",resources=hncconfigurations,verbs=create;update;delete,versions=v1alpha2,name=hncconfigurations.hnc.x-k8s.io

type HNCConfig struct {
	Log       logr.Logger
	Forest    *forest.Forest
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
		if req.Name == api.HNCConfigSingleton {
			return deny(metav1.StatusReasonForbidden, "Deleting the 'config' object is forbidden")
		} else {
			// We allow deleting other objects. We should never enter this case with the CRD validation. We introduced
			// the CRD validation in v0.6. Before that, it was protected by the validation controller. If users somehow
			// bypassed the validation controller and created objects of other names, those objects would still have an
			// obsolete condition and we will allow users to delete the objects.
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

	// Lastly, check if changing a type to "Propagate" mode would cause
	// overwriting user-created objects.
	return c.checkForest(inst)
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

	return allow("")
}

func (c *HNCConfig) checkForest(inst *api.HNCConfiguration) admission.Response {
	c.Forest.Lock()
	defer c.Forest.Unlock()

	// Get types that are changed from other modes to "Propagate" mode.
	gvks := c.getNewPropagateTypes(inst)

	// Check if user-created objects would be overwritten by these mode changes.
	for gvk, _ := range gvks {
		conflicts := c.checkConflictsForGVK(gvk)
		if len(conflicts) != 0 {
			msg := fmt.Sprintf("Cannot update configuration because setting type %q to 'Propagate' mode would overwrite user-created object(s):\n", gvk)
			msg += strings.Join(conflicts, "\n")
			msg += "\nTo fix this, please rename or remove the conflicting objects first."
			return deny(metav1.StatusReasonConflict, msg)
		}
	}

	return allow("")
}

// checkConflictsForGVK looks for conflicts from top down for each tree.
func (c *HNCConfig) checkConflictsForGVK(gvk schema.GroupVersionKind) []string {
	conflicts := []string{}
	for _, ns := range c.Forest.GetRoots() {
		conflicts = append(conflicts, c.checkConflictsForTree(gvk, ancestorObjects{}, ns)...)
	}
	return conflicts
}

func (c *HNCConfig) checkConflictsForTree(gvk schema.GroupVersionKind, ao ancestorObjects, ns *forest.Namespace) []string {
	conflicts := []string{}
	objs := ao.copy()
	for _, o := range ns.GetSourceObjects(gvk) {
		onm := o.GetName()
		objs.add(onm, ns)
		// If there are more than just the one (current) namespace we just added,
		// there may be conflicts.
		if objs.hasConflict(onm) {
			// Currently the top conflicting ancestor would overwrite the object.
			// TODO: check if this is a real conflict
			conflicts = append(conflicts, fmt.Sprintf("  Object %q in namespace %q would overwrite the one in %q", onm, objs.top(onm), ns.Name()))
		}
	}
	// This is cycle-free and safe because we only start the
	// "checkConflictsForTree" from roots in the forest with cycles omitted and
	// it's impossible to get cycles from non-root.
	for _, cnm := range ns.ChildNames() {
		cns := c.Forest.Get(cnm)
		conflicts = append(conflicts, c.checkConflictsForTree(gvk, objs, cns)...)
	}
	return conflicts
}

// getNewPropagateTypes returns a set of types that are changed from other modes
// to `Propagate` mode.
func (c *HNCConfig) getNewPropagateTypes(inst *api.HNCConfiguration) gvkSet {
	// Get all "Propagate" mode types in the new configuration.
	newPts := gvkSet{}
	for _, t := range inst.Spec.Types {
		if t.Mode == api.Propagate {
			gvk := schema.FromAPIVersionAndKind(t.APIVersion, t.Kind)
			newPts[gvk] = true
		}
	}

	// Remove all existing "Propagate" mode types in the forest (current configuration).
	for _, t := range c.Forest.GetTypeSyncers() {
		if t.GetMode() == api.Propagate && newPts[t.GetGVK()] {
			delete(newPts, t.GetGVK())
		}
	}

	return newPts
}

// ancestorObjects maps an object name to the ancestor namespace(s) in which
// it's defined.
type ancestorObjects map[string][]string

func (a ancestorObjects) copy() ancestorObjects {
	copy := ancestorObjects{}
	for k, v := range a {
		copy[k] = v
	}
	return copy
}

func (a ancestorObjects) add(onm string, ns *forest.Namespace) {
	a[onm] = append(a[onm], ns.Name())
}

func (a ancestorObjects) hasConflict(onm string) bool {
	return len(a[onm]) > 1
}

func (a ancestorObjects) top(onm string) string {
	return a[onm][0]
}

// realGVKValidator implements gvkValidator, and is not used during unit tests.
type realGVKValidator struct {
	config *rest.Config
}

// Exists validates if a given GVK exists in the apiserver. The function uses a
// discovery client to find a matching resource for the GVK. It returns an error
// if it doesn't exist and returns nil if it does exist.
func (r *realGVKValidator) Exists(ctx context.Context, gvk schema.GroupVersionKind) error {
	dc, err := discovery.NewDiscoveryClientForConfig(r.config)
	if err != nil {
		// Fail close if we cannot create discovery client.
		return err
	}

	resources, err := dc.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		// No matching GV found.
		return err
	}

	// The GV exists. Look for the matching kind now.
	for _, resource := range resources.APIResources {
		if resource.Kind == gvk.Kind {
			return nil
		}
	}

	// No matching kind. Use the same error message when the GV is not found above.
	return errors.NewBadRequest("the server could not find the requested resource")
}

func (c *HNCConfig) InjectConfig(cf *rest.Config) error {
	c.validator = &realGVKValidator{config: cf}
	return nil
}

func (c *HNCConfig) InjectDecoder(d *admission.Decoder) error {
	c.decoder = d
	return nil
}
