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

// objSet is a set of namespaced objects. It also stores if the object has been
// checked for naming conflict.
type objSet map[string]inNamespace
type inNamespace struct {
	namespace string
	checked   bool
}

// conflicts stores object naming conflicts by GVK, object name and groups of
// conflicting namespaces. The namespaces are grouped by the first ancestor.
type conflicts map[schema.GroupVersionKind]map[string]affectedNamespaces
type affectedNamespaces map[string][]string

func (c *HNCConfig) checkForest(inst *api.HNCConfiguration) admission.Response {
	c.Forest.Lock()
	defer c.Forest.Unlock()

	// Get types that are changed from other modes to "Propagate" mode.
	gvks := c.getNewPropagateTypes(inst)

	// Check if user-created objects would be overwritten by these mode changes.
	confs := conflicts{}
	for gvk, _ := range gvks {
		// We will keep one object set for each type, because objects with the same
		// name but different types won't affect each other.
		obs := objSet{}

		// Traverse all the namespaces.
		for _, nnm := range c.Forest.GetNamespaceNames() {
			ns := c.Forest.Get(nnm)

			// Traverse all the original objects in each namespace. Please note that
			// if there's no object reconciler for this type (the type has never changed
			// to any mode other than "Ignore"), we will get nothing from the forest.
			// TODO update the conflictingAnsName() func to reflect the HNC exceptions
			//  (still under implementation) specified by the object labels. See
			//  https://docs.google.com/document/d/17J8icBEDvLLoPT4kQ4ArZcCerRweDY-TpJ48DJKpHJ0/edit#heading=h.wyw4gg116rw7
			for _, obnm := range ns.GetObjectNames(gvk) {
				// Check if an object of the same name exists. If not, add it to the set,
				// but we don't check if there's any conflict yet since we can leave it
				// until we detect there are more than just one object with the same name.
				if exObj, exist := obs[obnm]; !exist {
					obs[obnm] = inNamespace{namespace: nnm}
				} else {
					// If the object name already exists, let's check if there's any
					// conflict in the ancestors for both objects. We only check ancestors
					// because if there's a conflict, one must be the other's ancestor.
					if !exObj.checked {
						// If we haven't checked conflicts in ancestors for the existing one,
						// check it and record it, so we don't need to do it again if we find
						// more objects with the same name later.
						exnnm := exObj.namespace
						obs[obnm] = inNamespace{namespace: exnnm, checked: true}
						addConflictIfAny(confs, gvk, obnm, c.conflictingAnsName(obnm, exnnm, gvk), exnnm)
					}

					// Check naming conflict for this object too.
					addConflictIfAny(confs, gvk, obnm, c.conflictingAnsName(obnm, nnm, gvk), nnm)
				}
			}
		}
	}

	if len(confs) != 0 {
		return deny(metav1.StatusReasonConflict, printMsgWithConflicts(confs))
	}
	return allow("")
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

// conflictingAnsName returns the ancestor namespace name if there's any conflict.
func (c *HNCConfig) conflictingAnsName(obnm, nnm string, gvk schema.GroupVersionKind) string {
	ns := c.Forest.Get(nnm)
	for _, n := range ns.AncestryNames() {
		// Exclude the original objects in this namespace
		if n == nnm {
			continue
		}
		for _, aobnm := range c.Forest.Get(n).GetObjectNames(gvk) {
			if aobnm == obnm {
				return n
			}
		}
	}
	return ""
}

// addConflictIfAny adds a new conflict if there is one.
func addConflictIfAny(confs conflicts, gvk schema.GroupVersionKind, obnm, canm, nnm string) {
	// If there's no conflicting ancestor, do nothing.
	if canm == "" {
		return
	}

	if _, ok := confs[gvk]; !ok {
		confs[gvk] = make(map[string]affectedNamespaces)
	}
	if _, ok := confs[gvk][obnm]; !ok {
		confs[gvk][obnm] = affectedNamespaces{}
	}
	// If the descendant namespace is already in the list, do not add it again.
	ls := confs[gvk][obnm][canm]
	for _, nm := range ls {
		if nm == nnm {
			return
		}
	}
	confs[gvk][obnm][canm] = append(ls, nnm)
}

// printMsgWithConflicts prints conflicts into something like this:
// Cannot update configuration because setting types to 'Propagate' mode would overwrite user-created object(s):
// * Type: "/v1, Kind=Secret", Conflict(s):
//   - "my-creds2" in namespace "acme-org" would overwrite the one(s) in [team-b].
//   - "my-creds" in namespace "acme-org" would overwrite the one(s) in [team-a, team-b].
//   - "my-creds" in namespace "bcme-org" would overwrite the one(s) in [team-c].
//To fix this, please rename or remove the conflicting objects first.
func printMsgWithConflicts(conflicts conflicts) string {
	msg := "Cannot update configuration because setting types to 'Propagate' mode would overwrite user-created object(s):\n"
	for t, tconfs := range conflicts {
		msg += fmt.Sprintf(" * Type: %q, Conflict(s):\n", t)
		for obnm, affnms := range tconfs {
			for anm, dnms := range affnms {
				msg += fmt.Sprintf("   - %q in namespace %q would overwrite the one(s) in [", obnm, anm)
				msg += strings.Join(dnms, ", ")
				msg += "].\n"
			}
		}
	}
	msg += "To fix this, please rename or remove the conflicting objects first."
	return msg
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
