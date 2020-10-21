package validators

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/reconcilers"

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
	Log        logr.Logger
	Forest     *forest.Forest
	translator grTranslator
	decoder    *admission.Decoder
}

// grTranslator checks if a resource exists. The check should typically be
// performed against the apiserver, but need to be stubbed out in unit testing.
type grTranslator interface {
	// GVKFor returns the mapping GVK for a GR if it exists in the apiserver. If
	// it doesn't exist, return the error.
	GVKFor(gr schema.GroupResource) (schema.GroupVersionKind, error)
}

type gvkSet map[schema.GroupVersionKind]api.SynchronizationMode

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
	ts := gvkSet{}
	// Convert all valid types from GR to GVK. If any type is invalid, e.g. not
	// exist in the apiserver, wrong configuration, deny the request.
	if rp := c.validateTypes(inst, ts); !rp.Allowed {
		return rp
	}

	// Lastly, check if changing a type to "Propagate" mode would cause
	// overwriting user-created objects.
	return c.checkForest(inst, ts)
}

func (c *HNCConfig) validateTypes(inst *api.HNCConfiguration, ts gvkSet) admission.Response {
	roleExist := false
	roleBindingExist := false
	for _, r := range inst.Spec.Resources {
		// Validate the type exists in the apiserver. If yes, convert GR to GVK. We
		// use GVK because we will need to checkForest() later to avoid source
		// overwriting conflict (forest uses GVK as the key for object reconcilers).
		gr := schema.GroupResource{Group: r.Group, Resource: r.Resource}
		gvk, err := c.translator.GVKFor(gr)
		if err != nil {
			return deny(metav1.StatusReasonInvalid,
				fmt.Sprintf("Cannot find the %s in the apiserver with error: %s", gr, err.Error()))
		}

		// Validate if the configuration of a type already exists. Each type should
		// only have one configuration.
		if _, exists := ts[gvk]; exists {
			return deny(metav1.StatusReasonInvalid, fmt.Sprintf("Duplicate configurations for %s", gr))
		}
		ts[gvk] = r.Mode

		// ValidateThe mode of Role and RoleBinding should be either unset or set to
		// the propagate mode.
		if r.Group == api.RBACGroup && r.Resource == api.RoleResource {
			roleExist = true
			if r.Mode != api.Propagate && r.Mode != "" {
				return deny(metav1.StatusReasonInvalid, fmt.Sprintf("Invalid mode of %s; current mode: %s; expected mode %s", r.Resource, r.Mode, api.Propagate))
			}
		}

		if r.Group == api.RBACGroup && r.Resource == api.RoleBindingResource {
			roleBindingExist = true
			if r.Mode != api.Propagate && r.Mode != "" {
				return deny(metav1.StatusReasonInvalid, fmt.Sprintf("Invalid mode of %s; current mode: %s; expected mode %s", r.Resource, r.Mode, api.Propagate))
			}
		}
	}
	// Validate Role and RoleBinding exists in the Spec.
	// TODO this validation will be removed when we remove the configuration from
	//  Spec and only show them in the status.
	if !roleExist {
		return deny(metav1.StatusReasonInvalid, "Configuration for Role is missing")
	}
	if !roleBindingExist {
		return deny(metav1.StatusReasonInvalid, "Configuration for RoleBinding is missing")
	}
	return allow("")
}

func (c *HNCConfig) checkForest(inst *api.HNCConfiguration, ts gvkSet) admission.Response {
	c.Forest.Lock()
	defer c.Forest.Unlock()

	// Get types that are changed from other modes to "Propagate" mode.
	gvks := c.getNewPropagateTypes(ts)

	// Check if user-created objects would be overwritten by these mode changes.
	for gvk := range gvks {
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
func (c *HNCConfig) getNewPropagateTypes(ts gvkSet) gvkSet {
	// Get all "Propagate" mode types in the new configuration.
	newPts := gvkSet{}
	for gvk, mode := range ts {
		if mode == api.Propagate {
			newPts[gvk] = api.Propagate
		}
	}

	// Remove all existing "Propagate" mode types in the forest (current configuration).
	for _, t := range c.Forest.GetTypeSyncers() {
		_, exist := newPts[t.GetGVK()]
		if t.GetMode() == api.Propagate && exist {
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

// realGRTranslator implements grTranslator, and is not used during unit tests.
type realGRTranslator struct {
	config *rest.Config
}

// GVKFor searches a given GR in the apiserver and returns the mapping GVK. It
// returns an empty GVK and an error if the GR doesn't exist.
func (r *realGRTranslator) GVKFor(gr schema.GroupResource) (schema.GroupVersionKind, error) {
	allRes, err := reconcilers.GetAllResources(r.config)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}

	return reconcilers.GVKFor(gr, allRes)
}

func (c *HNCConfig) InjectConfig(cf *rest.Config) error {
	c.translator = &realGRTranslator{config: cf}
	return nil
}

func (c *HNCConfig) InjectDecoder(d *admission.Decoder) error {
	c.decoder = d
	return nil
}
