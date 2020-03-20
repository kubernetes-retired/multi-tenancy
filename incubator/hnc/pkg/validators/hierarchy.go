package validators

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	authnv1 "k8s.io/api/authentication/v1"
	authzv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

const (
	// HierarchyServingPath is where the validator will run. Must be kept in sync with the
	// kubebuilder marker below.
	HierarchyServingPath = "/validate-hnc-x-k8s-io-v1alpha1-hierarchyconfigurations"
)

// Note: the validating webhook FAILS CLOSED. This means that if the webhook goes down, all further
// changes to the hierarchy are forbidden. However, new objects will still be propagated according
// to the existing hierarchy (unless the reconciler is down too).
//
// +kubebuilder:webhook:path=/validate-hnc-x-k8s-io-v1alpha1-hierarchyconfigurations,mutating=false,failurePolicy=fail,groups="hnc.x-k8s.io",resources=hierarchyconfigurations,verbs=create;update,versions=v1alpha1,name=hierarchyconfigurations.hnc.x-k8s.io

type Hierarchy struct {
	Log     logr.Logger
	Forest  *forest.Forest
	authz   authzClient
	decoder *admission.Decoder
}

// authzClient represents the authz checks that should typically be performed against the apiserver,
// but need to be stubbed out during unit testing.
type authzClient interface {
	// IsAdmin takes a UserInfo and the name of a namespace, and returns true if the user is an admin
	// of that namespace (ie, can update the hierarchical config).
	IsAdmin(ctx context.Context, ui *authnv1.UserInfo, nnm string) (bool, error)
}

// request defines the aspects of the admission.Request that we care about.
type request struct {
	hc *api.HierarchyConfiguration
	ui *authnv1.UserInfo
}

// Handle implements the validation webhook.
//
// During updates, the validator currently ignores the existing state of the object (`oldObject`).
// The reason is that most of the checks being performed are on the state of the entire forest, not
// on any one object, so having the _very_ latest information on _one_ object doesn't really help
// us. That is, we're basically forced to assume that the in-memory forest is fully up-to-date.
//
// Obviously, there are times when this assumption will be incorrect - for example, when the HNC is
// just starting up, or perhaps if there have been a lot of changes made very quickly that the
// reconciler has't caught up with yet. In such cases, this validator can produce both false
// negatives (legal changes are incorrectly rejected) or false positives (illegal changes are
// mistakenly allowed).  False negatives can easily be retried and so are not a significant problem,
// since (by definition) we expect the problem to be transient.
//
// False positives are a more serious concern, but the reconciler has been designed to assume that
// the validator is _never_ running, and any illegal configuration that makes it into K8s will
// simply be reported via HierarchyConfiguration.Status.Conditions. It's the admins'
// responsibilities to monitor these conditions and ensure that, transient exceptions aside, all
// namespaces are condition-free.
func (v *Hierarchy) Handle(ctx context.Context, req admission.Request) admission.Response {
	decoded, err := v.decodeRequest(req)
	log := v.Log.WithValues("ns", decoded.hc.ObjectMeta.Namespace, "user", decoded.ui.Username)

	if err != nil {
		log.Error(err, "Couldn't decode request")
		return deny(metav1.StatusReasonBadRequest, err.Error())
	}

	resp := v.handle(ctx, log, decoded)
	log.Info("Handled", "allowed", resp.Allowed, "code", resp.Result.Code, "reason", resp.Result.Reason, "message", resp.Result.Message)
	return resp
}

// handle implements the non-boilerplate logic of this validator, allowing it to be more easily unit
// tested (ie without constructing a full admission.Request).
//
// This follows the standard HNC pattern of:
// - Load a bunch of stuff from the apiserver
// - Lock the forest and do all checks
// - Finish up with the apiserver (although we just run _additional_ checks, we don't modify things)
//
// This minimizes the amount of time that the forest is locked, allowing different threads to
// proceed in parallel.
func (v *Hierarchy) handle(ctx context.Context, log logr.Logger, req *request) admission.Response {
	// Early exit: the HNC SA can do whatever it wants. This is because if an illegal HC already
	// exists on the K8s server, we need to be able to update its status even though the rest of the
	// object wouldn't pass legality. We should probably only give the HNC SA the ability to modify
	// the _status_, though. TODO: https://github.com/kubernetes-sigs/multi-tenancy/issues/80.
	if isHNCServiceAccount(req.ui) {
		log.Info("Allowed change by HNC SA")
		return allow("HNC SA")
	}

	// Verify the HC is legal in isolation (i.e., before checking the rest of the forest)
	resp := checkConfig(req.hc)
	if !resp.Allowed {
		return resp
	}

	// Do all checks that require holding the in-memory lock. Generate a list of authz checks we
	// should perform once the lock is released.
	authzReqs, resp := v.checkForest(req.hc)
	if !resp.Allowed {
		return resp
	}

	// Ensure the user has the required permissions to make the change.
	return v.checkAuthz(ctx, req.ui, authzReqs)
}

// checkForest validates that the request is allowed based on the current in-memory state of the
// forest. If it is, it returns a list of namespaces that the user needs to be authorized to update
// in order to be allowed to make the change; these checks are executed _after_ the in-memory lock
// is released.
func (v *Hierarchy) checkForest(hc *api.HierarchyConfiguration) ([]authzReq, admission.Response) {
	v.Forest.Lock()
	defer v.Forest.Unlock()

	// Load stuff from the forest
	ns := v.Forest.Get(hc.ObjectMeta.Namespace)
	curParent := ns.Parent()
	newParent := v.Forest.Get(hc.Spec.Parent)

	resp := v.checkParent(ns, curParent, newParent)
	if !resp.Allowed {
		return nil, resp
	}

	resp = v.checkRequiredChildren(ns, hc.Spec.RequiredChildren)
	if !resp.Allowed {
		return nil, resp
	}

	// The structure looks good. Get the list of namespaces we need authz checks on.
	return v.needAuthzOn(curParent, newParent), allow("")
}

// checkParent validates if the parent is legal based on the current in-memory state of the forest.
func (v *Hierarchy) checkParent(ns, curParent, newParent *forest.Namespace) admission.Response {
	if curParent == newParent {
		return allow("parent unchanged")
	}

	// non existence of parent namespace -> not allowed
	if newParent != nil && !newParent.Exists() {
		return deny(metav1.StatusReasonForbidden, "The requested parent "+newParent.Name()+" does not exist")
	}

	// Is this change structurally legal? Note that this can "leak" information about the hierarchy
	// since we haven't done our authz checks yet. However, the fact that they've gotten this far
	// means that the user has permission to update the _current_ namespace, which means they also
	// have visibility into its ancestry and descendents, and this check can only fail if the new
	// parent conflicts with something in the _existing_ hierarchy.
	if reason := ns.CanSetParent(newParent); reason != "" {
		return deny(metav1.StatusReasonConflict, "Illegal parent: "+reason)
	}

	// Prevent changing parent of an owned child
	if ns.Owner != "" && ns.Owner != newParent.Name() {
		reason := fmt.Sprintf("Cannot set the parent of %q to %q because it's a self-serve subnamespace of %q", ns.Name(), newParent.Name(), ns.Owner)
		return deny(metav1.StatusReasonConflict, "Illegal parent: "+reason)
	}

	return allow("")
}

func (v *Hierarchy) checkRequiredChildren(ns *forest.Namespace, requiredChildren []string) admission.Response {
	for _, child := range requiredChildren {
		cns := v.Forest.Get(child)
		// A newly-created requiredChild is always valid.
		if !cns.Exists() {
			continue
		}
		// If this is already a child, or is about to be, no problem.
		if cns.Parent() == ns || (cns.Parent() == nil && cns.Owner == ns.Name()) {
			continue
		}
		reason := fmt.Sprintf("Cannot set %q as the required child of %q because it already exists and is not a child of %q", cns.Name(), ns.Name(), ns.Name())
		return deny(metav1.StatusReasonConflict, "Illegal requiredChild: "+reason)
	}

	return allow("")
}

// authzReq represents a request for authorization
type authzReq struct {
	nnm    string // the namespace the user needs to be authorized to modify
	reason string // the reason we're checking it (for logs and error messages)
}

// needAuthzOn returns the namespaces that the user must be authorized to update in order to make
// this change. It must be called while the forest lock is held.
//
// This method is a bit verbose; I've tried to optimize readability over conciseness since it's a
// tricky bit of code to understand.
func (v *Hierarchy) needAuthzOn(curParent, newParent *forest.Namespace) []authzReq {
	// Get the ancestry chain of both parents (see getExistingAncestry for details of what we actually
	// return).
	//
	// TODO: if the new parent doesn't exist yet, only allow it if the user has permission to create
	// it. https://github.com/kubernetes-sigs/multi-tenancy/issues/158.
	curChain := v.getExistingAncestry(curParent)
	newChain := v.getExistingAncestry(newParent)

	// No (valid) current or new ancestors -> nothing to check.
	if len(curChain) == 0 && len(newChain) == 0 {
		return nil
	}

	// If only one of them exists, return that one. If they both exist, but have different roots, add
	// both of them. Note that we've already covered the case where _neither_ exists, so if one
	// doesn't exist, the other certainly does.
	if len(curChain) == 0 {
		return []authzReq{{nnm: newChain[len(newChain)-1], reason: "proposed parent"}}
	}
	if len(newChain) == 0 {
		return []authzReq{{nnm: curChain[0], reason: "root ancestor of the current parent"}}
	}

	// If they don't share any common ancestors, return them both (trying to factor out the code to
	// create the requests actually made this function harder to read, IMO).
	if curChain[0] != newChain[0] {
		return []authzReq{
			{nnm: curChain[0], reason: "root ancestor of the current parent"},
			{nnm: newChain[len(newChain)-1], reason: "proposed parent"},
		}
	}

	// There's at least one common ancestor; find the most recent one and return it.
	mrca := curChain[0]
	for i := 1; i < len(curChain) && i < len(newChain); i++ {
		if curChain[i] != newChain[i] {
			break
		}
		mrca = curChain[i]
	}
	return []authzReq{{
		nnm:    mrca,
		reason: fmt.Sprintf("most recent common ancestor of current parent %s and proposed parent %s", curParent.Name(), newParent.Name()),
	}}
}

// getExistingAncestry returns the ancestry of the given namespace, with all nonexistent namespaces
// filtered out. We need to compare the ancestry of the two parents so we can find the most recent
// common ancestor and do authz checks on it. However, since we can assign parents before they
// exist, the MRCA might not actually exist yet, which means that K8s obviously can't do an authz
// check on it yet.
//
// It's even trickier than you might think: the ancestry chain can actually contain gaps! For
// example, namespace A might have a required child B which hasn't been created yet (for some
// reason), while namespace C lists namespace B as its parent. In this case, A and C exist, but not
// B. In this case, we should probably run the check on A, since it's admins are obviously supposed
// to have control over C and its descendents.
//
// Alternatively, B and C might both exist, and A might not. In this case, we can't run the check on
// A, so we do the best we can: we should run the check on B.
//
// We can solve both of these cases by simply filtering out the missing namespaces.
func (v *Hierarchy) getExistingAncestry(ns *forest.Namespace) []string {
	chain := ns.AncestryNames(nil) // Returns empty slice if ns is nil.
	existing := []string{}
	for _, anm := range chain {
		if v.Forest.Get(anm).Exists() {
			existing = append(existing, anm)
		}
	}
	return existing
}

// checkAuthz executes the list of requested checks.
func (v *Hierarchy) checkAuthz(ctx context.Context, ui *authnv1.UserInfo, reqs []authzReq) admission.Response {
	if v.authz == nil {
		return allow("") // unit test; TODO put in fake
	}

	// TODO: parallelize?
	for _, req := range reqs {
		allowed, err := v.authz.IsAdmin(ctx, ui, req.nnm)

		// Interpret the result
		if err != nil {
			return deny(metav1.StatusReasonUnknown, fmt.Sprintf("while checking authz for %s, the %s: %s", req.nnm, req.reason, err))
		}

		if !allowed {
			return deny(metav1.StatusReasonUnauthorized, fmt.Sprintf("User %s is not authorized to modify the subtree of %s, which is the %s",
				ui.Username, req.nnm, req.reason))
		}
	}

	return allow("")
}

// decodeRequest gets the information we care about into a simple struct that's easy to both a) use
// and b) factor out in unit tests.
func (v *Hierarchy) decodeRequest(in admission.Request) (*request, error) {
	hc := &api.HierarchyConfiguration{}
	err := v.decoder.Decode(in, hc)
	if err != nil {
		return nil, err
	}

	return &request{
		hc: hc,
		ui: &in.UserInfo,
	}, nil
}

// isHNCServiceAccount is inspired by isGKServiceAccount from open-policy-agent/gatekeeper.
func isHNCServiceAccount(user *authnv1.UserInfo) bool {
	if user == nil {
		// useful for unit tests
		return false
	}

	ns, found := os.LookupEnv("POD_NAMESPACE")
	if !found {
		ns = "hnc-system"
	}
	saGroup := fmt.Sprintf("system:serviceaccounts:%s", ns)
	for _, g := range user.Groups {
		if g == saGroup {
			return true
		}
	}
	return false
}

// checkConfig is checking whether namespaces in the hierarchy configuration meet kubernetes requirements.
// if required children's field contains invalid name, it returns admission response that reject namespaces creation.
func checkConfig(hc *api.HierarchyConfiguration) admission.Response {

	// Check if children names in requiredChildren field obey kubernetes namespace regex format.
	// invalidRCs accomodates illegal required child(RC) name(s).
	invalidRCs := []string{}
	for _, rc := range hc.Spec.RequiredChildren {
		if resp := validateNamespace(rc); resp != nil {
			invalidRCs = append(invalidRCs, rc)
		}
	}

	if len(invalidRCs) > 0 {
		return deny(metav1.StatusReasonBadRequest, fmt.Sprintf("The following required children are not valid namespace names: %s", strings.Join(invalidRCs, ", ")))
	}
	return allow("")
}

// validateNamespace validates a string is a valid namespace using apimachinery.
// https://godoc.org/k8s.io/apimachinery/pkg/util/validation#IsDNS1123Label
func validateNamespace(s string) []string {
	return validation.IsDNS1123Label(s)
}

func (v *Hierarchy) InjectClient(c client.Client) error {
	v.authz = &realAuthzClient{client: c}
	return nil
}

func (v *Hierarchy) InjectDecoder(d *admission.Decoder) error {
	v.decoder = d
	return nil
}

// realAuthzClient implements authzClient, and is not use during unit tests.
type realAuthzClient struct {
	client client.Client
}

// IsAdmin implements authzClient
func (r *realAuthzClient) IsAdmin(ctx context.Context, ui *authnv1.UserInfo, nnm string) (bool, error) {
	// Convert the Extra type
	authzExtra := map[string]authzv1.ExtraValue{}
	for k, v := range ui.Extra {
		authzExtra[k] = (authzv1.ExtraValue)(v)
	}

	// Construct the request
	sar := &authzv1.SubjectAccessReview{
		Spec: authzv1.SubjectAccessReviewSpec{
			ResourceAttributes: &authzv1.ResourceAttributes{
				Namespace: nnm,
				Verb:      "update",
				Group:     "hnc.x-k8s.io",
				Version:   "*",
				Resource:  "hierarchyconfigurations",
			},
			User:   ui.Username,
			Groups: ui.Groups,
			UID:    ui.UID,
			Extra:  authzExtra,
		},
	}

	// Call the server
	err := r.client.Create(ctx, sar)

	// Extract the interesting result
	return sar.Status.Allowed, err
}

// allow is a replacement for controller-runtime's admission.Allowed() that allows you to set the
// message (human-readable) as opposed to the reason (machine-readable).
func allow(msg string) admission.Response {
	return admission.Response{AdmissionResponse: admissionv1beta1.AdmissionResponse{
		Allowed: true,
		Result: &metav1.Status{
			Code:    0,
			Message: msg,
		},
	}}
}

// deny is a replacement for controller-runtime's admission.Denied() that allows you to set _both_ a
// human-readable message _and_ a machine-readable reason, and also sets the code correctly instead
// of hardcoding it to 403 Forbidden.
func deny(reason metav1.StatusReason, msg string) admission.Response {
	if reason != metav1.StatusReasonInvalid {
		return admission.Response{AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Code:    codeFromReason(reason),
				Message: msg,
				Reason:  reason,
			},
		}}
	} else {
		// metav1.StatusReasonInvalid shows the custom message in the Details field instead of
		// Message field of metav1.Status.
		return admission.Response{AdmissionResponse: admissionv1beta1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Code:   codeFromReason(reason),
				Reason: reason,
				Details: &metav1.StatusDetails{
					Causes: []metav1.StatusCause{
						{
							Message: msg,
						},
					},
				},
			},
		}}
	}
}

// codeFromReason implements the needed subset of
// https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#StatusReason
func codeFromReason(reason metav1.StatusReason) int32 {
	switch reason {
	case metav1.StatusReasonUnknown:
		return 500
	case metav1.StatusReasonUnauthorized:
		return 401
	case metav1.StatusReasonForbidden:
		return 403
	case metav1.StatusReasonConflict:
		return 409
	case metav1.StatusReasonBadRequest:
		return 400
	case metav1.StatusReasonInvalid:
		return 422
	case metav1.StatusReasonInternalError:
		return 500
	default:
		return 500
	}
}
