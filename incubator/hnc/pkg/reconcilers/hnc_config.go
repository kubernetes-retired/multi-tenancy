package reconcilers

import (
	"context"
	"fmt"
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

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

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
}

type gvkSet map[schema.GroupVersionKind]bool

// Reconcile sets up some basic variable and logs the Spec.
// TODO: Updates the comment above when adding more logic to the Reconcile method.
func (r *ConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	// Object reconcilers will trigger the config reconciler at the end of each object
	// reconciliation for updating the status of the `config` singleton. Sleep here so
	// that a batch of reconciliation requests issued by object reconcilers can be
	// treated as one request to avoid invoking the config reconciler very frequently.
	time.Sleep(3 * time.Second)

	ctx := context.Background()

	// Validate the singleton name.
	if err := r.validateSingletonName(ctx, req.NamespacedName.Name); err != nil {
		r.Log.Error(err, "Singleton name validation failed")
		return ctrl.Result{}, nil
	}

	inst, err := r.getSingleton(ctx)
	if err != nil {
		r.Log.Error(err, "Couldn't read singleton")
		return ctrl.Result{}, err
	}

	// Clear the existing the status because we will reconstruct the latest status.
	inst.Status.Conditions = nil
	inst.Status.Types = nil

	// Create or sync corresponding ObjectReconcilers, if needed.
	syncErr := r.syncObjectReconcilers(ctx, inst)

	// Sync the status for each type.
	r.syncTypeStatus(inst)

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
		Mode:              mode,
		Affected:          make(chan event.GenericEvent),
		AffectedNamespace: r.HierarchyConfigUpdates,
		PropagatedObjects: namespacedNameSet{},
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

func (r *ConfigReconciler) validateSingletonName(ctx context.Context, nm string) error {
	if nm == api.HNCConfigSingleton {
		return nil
	}

	nnm := types.NamespacedName{Name: nm}
	inst := &api.HNCConfiguration{}
	if err := r.Get(ctx, nnm, inst); err != nil {
		return err
	}

	msg := "Singleton name is wrong. It should be 'config'"
	condition := api.HNCConfigurationCondition{
		Code: api.CritSingletonNameInvalid,
		Msg:  msg,
	}
	inst.Status.Conditions = nil
	inst.Status.Conditions = append(inst.Status.Conditions, condition)

	if err := r.writeSingleton(ctx, inst); err != nil {
		return err
	}

	return fmt.Errorf("Error while validating singleton name: %s", msg)
}

// syncTypeStatus syncs Status.Types for types configured in the spec with
// object reconcilers and the forest.
func (r *ConfigReconciler) syncTypeStatus(inst *api.HNCConfiguration) {
	pojs := r.syncNumPropagatedObjects()
	sojs := r.syncNumSourceObjects()
	r.addTypesStatus(pojs, sojs, inst)
}

// syncNumSourceObjects computes the number of propagated objects for each type in the spec by syncing with
// object reconcilers.
func (r *ConfigReconciler) syncNumPropagatedObjects() map[schema.GroupVersionKind]int32 {
	pojs := map[schema.GroupVersionKind]int32{}
	for _, ts := range r.Forest.GetTypeSyncers() {
		gvk := ts.GetGVK()
		if r.activeGVKs[gvk] {
			pojs[gvk] = ts.GetNumPropagatedObjects()
			r.Log.Info("syncNumPropagatedObjects", "kind", gvk.Kind, "num", pojs[gvk])
		}
	}
	return pojs
}

// syncNumSourceObjects computes the number of source objects for each type in the spec by syncing with the
// forest.
func (r *ConfigReconciler) syncNumSourceObjects() map[schema.GroupVersionKind]int32 {
	sojs := map[schema.GroupVersionKind]int32{}
	nms := r.Forest.GetNamespaces()
	for _, ns := range nms {
		for gvk, _ := range r.activeGVKs {
			sojs[gvk] += int32(ns.GetNumOriginalObjects(gvk))
		}
	}
	return sojs
}

// addTypesStatus adds the NumPropagatedObjects and NumSourceObjects fields for a given GVK in the status.
// The method adds NumSourceObjects only for types in propagate mode because keeping track of the number of
// source objects for types in remove and ignore modes is out of the scope of HNC. The method adds
// NumPropagatedObjects for types in all modes.
func (r *ConfigReconciler) addTypesStatus(pojs map[schema.GroupVersionKind]int32,
	sobjs map[schema.GroupVersionKind]int32, inst *api.HNCConfiguration) {
	for _, ts := range r.Forest.GetTypeSyncers() {
		gvk := ts.GetGVK()
		mode := ts.GetMode()
		if r.activeGVKs[gvk] {
			apiVersion, kind := gvk.ToAPIVersionAndKind()
			soj := sobjs[gvk]
			poj := pojs[gvk]
			r.Log.Info("addTypesStatus", "kind", kind, "num", poj)
			if mode == api.Propagate || mode == "" {
				inst.Status.Types = append(inst.Status.Types, api.TypeSynchronizationStatus{
					APIVersion:           apiVersion,
					Kind:                 kind,
					NumPropagatedObjects: &poj,
					NumSourceObjects:     &soj,
				})
			} else {
				inst.Status.Types = append(inst.Status.Types, api.TypeSynchronizationStatus{
					APIVersion:           apiVersion,
					Kind:                 kind,
					NumPropagatedObjects: &poj,
				})
			}
		}
	}
}

// enqueueSingleton enqueues the `config` singleton to trigger the reconciliation
// of the singleton for a given reason . This occurs in a goroutine so the
// caller doesn't block; since the reconciler is never garbage-collected,
// this is safe.
func (r *ConfigReconciler) enqueueSingleton(log logr.Logger, reason string) {
	go func() {
		log.Info("Enqueuing for reconciliation", "reason", reason)
		// The watch handler doesn't care about anything except the metadata.
		inst := &api.HNCConfiguration{}
		inst.ObjectMeta.Name = api.HNCConfigSingleton
		r.Trigger <- event.GenericEvent{Meta: inst}
	}()
}

// SyncHNCConfigStatus is called by an object reconciler to trigger reconciliation of
// the 'config' singleton for updating the status.
func (r *ConfigReconciler) SyncHNCConfigStatus(log logr.Logger) {
	r.enqueueSingleton(log, "Sync NumPropagatedObjects in the status")
}

// SetupWithManager builds a controller with the reconciler.
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Whenever a CRD is created/updated, we will send a request to reconcile the
	// singleton again, in case the singleton has configuration for the resource.
	crdMapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			nnm := types.NamespacedName{
				Name: api.HNCConfigSingleton,
			}
			return []reconcile.Request{
				{NamespacedName: nnm},
			}
		})
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
	r.enqueueSingleton(r.Log, "Enforce reconciliation to create a default"+
		"HNCConfiguration singleton if it does not exist")

	// Informs the forest about the config reconciler so that it can be triggered
	// by object reconcilers for updating the status.
	r.Forest.AddConfigSyncer(r)
	return nil
}
