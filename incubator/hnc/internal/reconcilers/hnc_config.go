package reconcilers

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/config"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
)

// hnccrSingleton stores a pointer to the cluster-wide config reconciler so anyone can
// request that it updates itself (see UpdateHNCConfig, below).
var hnccrSingleton *ConfigReconciler

// ConfigReconciler is responsible for determining the HNC configuration from the HNCConfiguration CR,
// as well as ensuring all objects are propagated correctly when the HNC configuration changes.
// It can also set the status of the HNCConfiguration CR.
type ConfigReconciler struct {
	client.Client
	Log     logr.Logger
	Manager ctrl.Manager

	// Forest is the in-memory data structure that is shared with all other reconcilers.
	Forest *forest.Forest

	// Trigger is a channel of event.GenericEvent (see "Watching Channels" in
	// https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html)
	// that is used to enqueue the singleton to trigger reconciliation.
	Trigger chan event.GenericEvent

	// HierarchyConfigUpdates is a channel of events used to update hierarchy configuration changes performed by
	// ObjectReconcilers. It is passed on to ObjectReconcilers for the updates. The ConfigReconciler itself does
	// not use it.
	HierarchyConfigUpdates chan event.GenericEvent

	// activeGVKs contains GVKs that are configured in the Spec.
	activeGVKs gvkSet

	// enqueueReasons maintains the list of reasons we've been asked to update ourselves. Functionally,
	// it's just a boolean (nil or non-nil), everything else is just for logging.
	enqueueReasons     map[string]int
	enqueueReasonsLock sync.Mutex
}

// gvkSet keeps track of a group of unique GVKs.
type gvkSet map[schema.GroupVersionKind]bool

// checkPeriod is the period that the config reconciler checks if it needs to reconcile the
// `config` singleton.
const checkPeriod = 3 * time.Second

// Reconcile is the entrypoint to the reconciler.
func (r *ConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	// Validate the singleton name, and early exit if we're not validating the real one (note that the
	// bad singleton will have a condition set, but the main one will not be affected).
	if ok, err := r.validateSingletonName(ctx, req.NamespacedName.Name); !ok || err != nil {
		r.Log.Error(err, "An incorrectly-named HNC Config exists", "name", req.NamespacedName.Name)
		return ctrl.Result{}, err
	}

	// Load the config and clear its conditions so they can be reset.
	inst, err := r.getSingleton(ctx)
	if err != nil {
		r.Log.Error(err, "Couldn't read singleton")
		return ctrl.Result{}, err
	}
	inst.Status.Conditions = nil

	// Create or sync corresponding ObjectReconcilers, if needed.
	syncErr := r.syncObjectReconcilers(ctx, inst)

	// Set the status for each type.
	r.setTypeStatuses(inst)

	// Write back to the apiserver.
	if err := r.writeSingleton(ctx, inst); err != nil {
		r.Log.Error(err, "Couldn't write singleton")
		return ctrl.Result{}, err
	}

	// Retry reconciliation if there is a sync error.
	return ctrl.Result{}, syncErr
}

// getSingleton returns the singleton if it exists, or creates a default one if it doesn't.
func (r *ConfigReconciler) getSingleton(ctx context.Context) (*api.HNCConfiguration, error) {
	nnm := types.NamespacedName{Name: api.HNCConfigSingleton}
	inst := &api.HNCConfiguration{}
	if err := r.Get(ctx, nnm, inst); err != nil {
		if !errors.IsNotFound(err) {
			return nil, err
		}
	}

	r.validateSingleton(inst)
	return inst, nil
}

// validateSingleton sets the singleton name if it hasn't been set and ensures
// Role and RoleBinding have the default configuration.
func (r *ConfigReconciler) validateSingleton(inst *api.HNCConfiguration) {
	// It is possible that the singleton does not exist on the apiserver. In this
	// case its name hasn't been set yet.
	if inst.ObjectMeta.Name == "" {
		r.Log.Info(fmt.Sprintf("Setting the object name to be %s", api.HNCConfigSingleton))
		inst.ObjectMeta.Name = api.HNCConfigSingleton
	}
	r.validateRBACTypes(inst)
}

// validateRBACTypes set Role and RoleBindings to the default configuration if they are
// missing or having non-default configuration in the Spec.
func (r *ConfigReconciler) validateRBACTypes(inst *api.HNCConfiguration) {
	roleExists, roleBindingsExists := false, false

	// Check the mode of Role and RoleBinding. The mode can be either the default mode
	// or not set; otherwise, we will change the mode to the default mode.
	for i := 0; i < len(inst.Spec.Types); i++ {
		t := &inst.Spec.Types[i]
		if r.isRole(*t) {
			mode := config.GetDefaultRoleSpec().Mode
			if t.Mode != mode && t.Mode != "" {
				r.Log.Info(fmt.Sprintf("Invalid mode for Role. Changing the mode from %s to %s", t.Mode, mode))
				t.Mode = mode
			}
			roleExists = true
		} else if r.isRoleBinding(*t) {
			mode := config.GetDefaultRoleBindingSpec().Mode
			if t.Mode != mode && t.Mode != "" {
				r.Log.Info(fmt.Sprintf("Invalid mode for RoleBinding. Changing the mode from %s to %s", t.Mode, mode))
				t.Mode = mode
			}
			roleBindingsExists = true
		}
	}

	// If Role and/or RoleBinding do not exist in the configuration, we will insert
	// the default configuration in the spec for each of them.
	if !roleExists {
		r.Log.Info("Adding default configuration for Role")
		inst.Spec.Types = append(inst.Spec.Types, config.GetDefaultRoleSpec())
	}
	if !roleBindingsExists {
		r.Log.Info("Adding default configuration for RoleBinding")
		inst.Spec.Types = append(inst.Spec.Types, config.GetDefaultRoleBindingSpec())
	}
}

func (r *ConfigReconciler) isRole(t api.TypeSynchronizationSpec) bool {
	return t.APIVersion == "rbac.authorization.k8s.io/v1" && t.Kind == "Role"
}

func (r *ConfigReconciler) isRoleBinding(t api.TypeSynchronizationSpec) bool {
	return t.APIVersion == "rbac.authorization.k8s.io/v1" && t.Kind == "RoleBinding"
}

// writeSingleton creates a singleton on the apiserver if it does not exist.
// Otherwise, it updates existing singleton on the apiserver.
// We will write the singleton to apiserver even it is not changed because we assume this
// reconciler is called very infrequently and is not performance critical.
func (r *ConfigReconciler) writeSingleton(ctx context.Context, inst *api.HNCConfiguration) error {
	if inst.CreationTimestamp.IsZero() {
		r.Log.Info("Creating a default singleton on apiserver")
		if err := r.Create(ctx, inst); err != nil {
			r.Log.Error(err, "while creating on apiserver")
			return err
		}
	} else {
		r.Log.Info("Updating the singleton on apiserver")
		if err := r.Update(ctx, inst); err != nil {
			r.Log.Error(err, "while updating apiserver")
			return err
		}
	}

	return nil
}

// syncObjectReconcilers creates or syncs ObjectReconcilers.
//
// For newly added types in the HNC configuration, the method will create corresponding ObjectReconcilers and
// informs the in-memory forest about the newly created ObjectReconcilers. If a newly added type is misconfigured,
// the corresponding object reconciler will not be created. The method will not return error while creating
// ObjectReconcilers. Instead, any error will be written into the Status field of the singleton. This is
// intended to avoid infinite reconciliation when the type is misconfigured.
//
// If a type exists, the method will sync the mode of the existing object reconciler
// and update corresponding objects if needed. An error will be return to trigger reconciliation if sync fails.
func (r *ConfigReconciler) syncObjectReconcilers(ctx context.Context, inst *api.HNCConfiguration) error {
	// This method is guarded by the forest mutex.
	//
	// For creating an object reconciler, the mutex is guarding two actions: creating ObjectReconcilers for
	// the newly added types and adding the created ObjectReconcilers to the types list in the Forest (r.Forest.types).
	//
	// We use mutex to guard write access to the types list because the list is a shared resource between various
	// reconcilers. It is sufficient because both write and read access to the list are guarded by the same mutex.
	//
	// We use the forest mutex to guard ObjectReconciler creation together with types list modification so that the
	// forest cannot be changed by the HierarchyConfigReconciler until both actions are finished. If we do not use the
	// forest mutex to guard both actions, HierarchyConfigReconciler might change the forest structure and read the types list
	// from the forest for the object reconciliation, after we create ObjectReconcilers but before we write the
	// ObjectReconcilers to the types list. As a result, objects of the newly added types might not be propagated correctly
	// using the latest forest structure.
	//
	// For syncing an object reconciler, the mutex is guarding the read access to the `namespaces` field in the forest. The
	// `namespaces` field is a shared resource between various reconcilers.
	r.Forest.Lock()
	defer r.Forest.Unlock()

	if err := r.syncActiveReconcilers(ctx, inst); err != nil {
		return err
	}

	if err := r.syncRemovedReconcilers(ctx, inst); err != nil {
		return err
	}

	return nil
}

// syncActiveReconcilers syncs object reconcilers for types that are in the Spec. If an object reconciler exists, it sets
// its mode according to the Spec; otherwise, it creates the object reconciler.
func (r *ConfigReconciler) syncActiveReconcilers(ctx context.Context, inst *api.HNCConfiguration) error {
	// exist keeps track of existing types in the `config` singleton.
	exist := gvkSet{}
	r.activeGVKs = gvkSet{}
	for _, t := range inst.Spec.Types {
		// If there are multiple configurations of the same type, we will follow the first
		// configuration and ignore the rest.
		if !r.ensureNoDuplicateTypeConfigurations(inst, t, exist) {
			continue
		}
		gvk := schema.FromAPIVersionAndKind(t.APIVersion, t.Kind)
		r.activeGVKs[gvk] = true
		if ts := r.Forest.GetTypeSyncer(gvk); ts != nil {
			if err := ts.SetMode(ctx, t.Mode, r.Log); err != nil {
				return err // retry the reconciliation
			}
		} else {
			r.createObjectReconciler(gvk, t.Mode, inst)
		}
	}
	return nil
}

// ensureNoDuplicateTypeConfigurations checks whether the configuration of a type already exists and sets
// a condition in the status if the type exists. The method returns true if the type configuration does not exist;
// otherwise, returns false.
func (r *ConfigReconciler) ensureNoDuplicateTypeConfigurations(inst *api.HNCConfiguration, t api.TypeSynchronizationSpec,
	mp gvkSet) bool {
	gvk := schema.FromAPIVersionAndKind(t.APIVersion, t.Kind)
	if !mp[gvk] {
		mp[gvk] = true
		return true
	}
	specMsg := fmt.Sprintf("APIVersion: %s, Kind: %s, Mode: %s", t.APIVersion, t.Kind, t.Mode)
	r.Log.Info(fmt.Sprintf("Ignoring the configuration: %s", specMsg))
	condition := api.HNCConfigurationCondition{
		Code: api.MultipleConfigurationsForOneType,
		Msg: fmt.Sprintf("Ignore the configuration: %s because the configuration of the type already exists; "+
			"only the first configuration will be applied", specMsg),
	}
	inst.Status.Conditions = append(inst.Status.Conditions, condition)
	return false
}

// syncRemovedReconcilers sets object reconcilers to "ignore" mode for types that are removed from the Spec.
func (r *ConfigReconciler) syncRemovedReconcilers(ctx context.Context, inst *api.HNCConfiguration) error {
	// If a type exists in the forest but not exists in the Spec, we will
	// set the mode of corresponding object reconciler to "ignore".
	// TODO: Ideally, we should shut down the corresponding object
	// reconciler. Gracefully terminating an object reconciler is still under
	// development (https://github.com/kubernetes-sigs/controller-runtime/issues/764).
	// We will revisit the code below once the feature is released.
	for _, ts := range r.Forest.GetTypeSyncers() {
		exist := false
		for _, t := range inst.Spec.Types {
			if ts.GetGVK() == schema.FromAPIVersionAndKind(t.APIVersion, t.Kind) {
				exist = true
				break
			}
		}
		if exist {
			continue
		}
		// The type does not exist in the Spec. Ignore subsequent reconciliations.
		r.Log.Info("Type config removed", "gvk", ts.GetGVK())
		if err := ts.SetMode(ctx, api.Ignore, r.Log); err != nil {
			return err // retry the reconciliation
		}
	}
	return nil
}

// createObjectReconciler creates an ObjectReconciler for the given GVK and informs forest about the reconciler.
func (r *ConfigReconciler) createObjectReconciler(gvk schema.GroupVersionKind, mode api.SynchronizationMode, inst *api.HNCConfiguration) {
	r.Log.Info("Creating an object reconciler", "gvk", gvk, "mode", mode)

	// After upgrading sigs.k8s.io/controller-runtime version to v0.5.0, we can create
	// reconciler successfully even when the resource does not exist in the cluster.
	// Therefore, we explicitly check if the resource exists before creating the
	// reconciler.
	_, err := r.Manager.GetRESTMapper().RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		r.Log.Error(err, "Error while trying to get resource", "gvk", gvk)
		r.writeObjectReconcilerCreationFailedCondition(inst, gvk, err)
		return
	}

	or := &ObjectReconciler{
		Client:            r.Client,
		Log:               ctrl.Log.WithName("reconcilers").WithName(gvk.Kind),
		Forest:            r.Forest,
		GVK:               gvk,
		Mode:              GetValidateMode(mode, r.Log),
		Affected:          make(chan event.GenericEvent),
		AffectedNamespace: r.HierarchyConfigUpdates,
		propagatedObjects: namespacedNameSet{},
	}

	// TODO: figure out MaxConcurrentReconciles option - https://github.com/kubernetes-sigs/multi-tenancy/issues/291
	if err := or.SetupWithManager(r.Manager, 10); err != nil {
		r.Log.Error(err, "Error while trying to create ObjectReconciler", "gvk", gvk)
		r.writeObjectReconcilerCreationFailedCondition(inst, gvk, err)
		return
	}

	// Informs the in-memory forest about the new reconciler by adding it to the types list.
	r.Forest.AddTypeSyncer(or)
}

func (r *ConfigReconciler) writeObjectReconcilerCreationFailedCondition(inst *api.HNCConfiguration,
	gvk schema.GroupVersionKind, err error) {
	condition := api.HNCConfigurationCondition{
		Code: api.ObjectReconcilerCreationFailed,
		Msg:  fmt.Sprintf("Couldn't create ObjectReconciler for type %s: %s", gvk, err),
	}
	inst.Status.Conditions = append(inst.Status.Conditions, condition)
}

// validateSingletonName tries to ensure we only have a single HNC Config object in the cluster. It
// returns true if the singleton name is correct, and false if there's a bad copy (in which case,
// the rest of the reconciler is skipped).
func (r *ConfigReconciler) validateSingletonName(ctx context.Context, nm string) (bool, error) {
	// If the name is expected, no problem.
	if nm == api.HNCConfigSingleton {
		return true, nil
	}

	// Otherwise, let's update whatever's in this wayward copy by setting a critical condition on it.
	nnm := types.NamespacedName{Name: nm}
	inst := &api.HNCConfiguration{}
	if err := r.Get(ctx, nnm, inst); err != nil {
		return false, err
	}

	msg := "Singleton name is wrong. It should be 'config'"
	condition := api.HNCConfigurationCondition{
		Code: api.CritSingletonNameInvalid,
		Msg:  msg,
	}
	inst.Status.Conditions = nil
	inst.Status.Conditions = append(inst.Status.Conditions, condition)

	return false, r.writeSingleton(ctx, inst)
}

// setTypeStatuses adds Status.Types for types configured in the spec. Only the status of
// types in `propagate` and `remove` modes will be recorded. The Status.Types
// is sorted in alphabetical order based on APIVersion and Kind.
func (r *ConfigReconciler) setTypeStatuses(inst *api.HNCConfiguration) {
	// We lock the forest here so that other reconcilers cannot modify the
	// forest while we are reading from the forest.
	r.Forest.Lock()
	defer r.Forest.Unlock()

	statuses := []api.TypeSynchronizationStatus{}
	for _, ts := range r.Forest.GetTypeSyncers() {
		// Don't output a status for any reconciler that isn't explicitly listed in the spec
		gvk := ts.GetGVK()
		if !r.activeGVKs[ts.GetGVK()] {
			continue
		}

		// Initialize status
		apiVersion, kind := gvk.ToAPIVersionAndKind()
		status := api.TypeSynchronizationStatus{
			APIVersion: apiVersion,
			Kind:       kind,
			Mode:       ts.GetMode(), // may be different from the spec if it's implicit
		}

		// Only add NumPropagatedObjects if we're not ignoring this type
		if ts.GetMode() != api.Ignore {
			numProp := ts.GetNumPropagatedObjects()
			status.NumPropagatedObjects = &numProp
		}

		// Only add NumSourceObjects if we are propagating objects of this type.
		if ts.GetMode() == api.Propagate {
			numSrc := 0
			nms := r.Forest.GetNamespaceNames()
			for _, nm := range nms {
				ns := r.Forest.Get(nm)
				numSrc += ns.GetNumOriginalObjects(gvk)
			}
			status.NumSourceObjects = &numSrc
		}

		// Record the status
		statuses = append(statuses, status)
	}

	// Alphabetize
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].APIVersion != statuses[j].APIVersion {
			return statuses[i].APIVersion < statuses[j].APIVersion
		}
		return statuses[i].Kind < statuses[i].Kind
	})

	// Record the final list
	inst.Status.Types = statuses
}

// requestReconcile records that the reconciler needs to be reinvoked.
func (r *ConfigReconciler) requestReconcile(reason string) {
	r.enqueueReasonsLock.Lock()
	defer r.enqueueReasonsLock.Unlock()

	if r.enqueueReasons == nil {
		r.enqueueReasons = map[string]int{}
	}
	r.enqueueReasons[reason]++
}

// periodicTrigger periodically checks if the `config` singleton needs to be reconciled and
// enqueues the `config` singleton for reconciliation, if needed.
func (r *ConfigReconciler) periodicTrigger() {
	// run forever
	for {
		time.Sleep(checkPeriod)
		r.triggerReconcileIfNeeded()
	}
}

func (r *ConfigReconciler) triggerReconcileIfNeeded() {
	r.enqueueReasonsLock.Lock()
	defer r.enqueueReasonsLock.Unlock()

	if r.enqueueReasons == nil {
		return
	}

	// Log all reasons
	for reason, count := range r.enqueueReasons {
		r.Log.Info("Updating HNCConfig", "reason", reason, "count", count)
	}

	// Clear the flag and actually trigger the reconcile.
	r.enqueueReasons = nil
	go func() {
		inst := &api.HNCConfiguration{}
		inst.ObjectMeta.Name = api.HNCConfigSingleton
		r.Trigger <- event.GenericEvent{Meta: inst}
	}()
}

// SetupWithManager builds a controller with the reconciler.
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Whenever a CRD is created/updated, we will send a request to reconcile the singleton again, in
	// case the singleton has configuration for the custom resource type.
	crdMapFn := handler.ToRequestsFunc(
		func(_ handler.MapObject) []reconcile.Request {
			return []reconcile.Request{{NamespacedName: types.NamespacedName{
				Name: api.HNCConfigSingleton,
			}}}
		})

	// Register the reconciler
	err := ctrl.NewControllerManagedBy(mgr).
		For(&api.HNCConfiguration{}).
		Watches(&source.Channel{Source: r.Trigger}, &handler.EnqueueRequestForObject{}).
		Watches(&source.Kind{Type: &v1beta1.CustomResourceDefinition{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: crdMapFn}).
		Complete(r)
	if err != nil {
		return err
	}

	// Create a default singleton if there is no singleton in the cluster by forcing
	// reconciliation to start.
	//
	// The cache used by the client to retrieve objects might not be populated
	// at this point. As a result, we cannot use r.Get() to determine the existence
	// of the singleton and then use r.Create() to create the singleton if
	// it does not exist. As a workaround, we decide to enforce reconciliation. The
	// cache is populated at the reconciliation stage. A default singleton will be
	// created during the reconciliation if there is no singleton in the cluster.
	r.requestReconcile("force initial reconcile")

	// Periodically checks if the config reconciler needs to reconcile and trigger the
	// reconciliation if needed, in case the status needs to be updated. This occurs
	// in a goroutine so the caller doesn't block; since the reconciler is never
	// garbage-collected, this is safe.
	go r.periodicTrigger()

	return nil
}
