package reconcilers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/handler"
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

	// Igniter is a channel of event.GenericEvent (see "Watching Channels" in
	// https://book-v1.book.kubebuilder.io/beyond_basics/controller_watches.html)
	// that is used to enqueue the singleton for initial reconciliation.
	Igniter chan event.GenericEvent

	// HierarchyConfigUpdates is a channel of events used to update hierarchy configuration changes performed by
	// ObjectReconcilers. It is passed on to ObjectReconcilers for the updates. The ConfigReconciler itself does
	// not use it.
	HierarchyConfigUpdates chan event.GenericEvent
}

// Reconcile sets up some basic variable and logs the Spec.
// TODO: Updates the comment above when adding more logic to the Reconcile method.
func (r *ConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()

	// Validate the singleton name.
	// TODO: Add a validating admission controller to prevent the problem in the first place.
	if err := r.validateSingletonName(ctx, req.NamespacedName.Name); err != nil {
		r.Log.Error(err, "Singleton name validation failed")
		return ctrl.Result{}, nil
	}

	inst, err := r.getSingleton(ctx)
	if err != nil {
		r.Log.Error(err, "Couldn't read singleton")
		return ctrl.Result{}, err
	}

	// TODO: Modify this and other reconcilers (e.g., hierarchy and object reconcilers) to
	// achieve the reconciliation.
	r.Log.Info("Reconciling cluster-wide HNC configuration")

	// Clear the existing conditions because we will reconstruct the latest conditions.
	inst.Status.Conditions = nil

	// Create or sync corresponding ObjectReconcilers, if needed.
	syncErr := r.syncObjectReconcilers(ctx, inst)

	// Write back to the apiserver.
	// TODO: Update HNCConfiguration.Status before writing the singleton back to the apiserver.
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

		// It doesn't exist - initialize it to a default value.
		inst.Spec = config.GetDefaultConfigSpec()
		inst.ObjectMeta.Name = api.HNCConfigSingleton
	}

	return inst, nil
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

	for _, t := range inst.Spec.Types {
		gvk := schema.FromAPIVersionAndKind(t.APIVersion, t.Kind)
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

// forceInitialReconcile forces reconciliation to start after setting up the
// controller with the manager. This is used to create a default singleton if
// there is no singleton in the cluster. This occurs in a goroutine so the
// caller doesn't block; since the reconciler is never garbage-collected,
// this is safe.
func (r *ConfigReconciler) forceInitialReconcile(log logr.Logger, reason string) {
	go func() {
		log.Info("Enqueuing for reconciliation", "reason", reason)
		// The watch handler doesn't care about anything except the metadata.
		inst := &api.HNCConfiguration{}
		inst.ObjectMeta.Name = api.HNCConfigSingleton
		r.Igniter <- event.GenericEvent{Meta: inst}
	}()
}

// SetupWithManager builds a controller with the reconciler.
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := ctrl.NewControllerManagedBy(mgr).
		For(&api.HNCConfiguration{}).
		Watches(&source.Channel{Source: r.Igniter}, &handler.EnqueueRequestForObject{}).
		Complete(r)
	if err != nil {
		return err
	}
	// Create a default singleton if there is no singleton in the cluster.
	//
	// The cache used by the client to retrieve objects might not be populated
	// at this point. As a result, we cannot use r.Get() to determine the existence
	// of the singleton and then use r.Create() to create the singleton if
	// it does not exist. As a workaround, we decide to enforce reconciliation. The
	// cache is populated at the reconciliation stage. A default singleton will be
	// created during the reconciliation if there is no singleton in the cluster.
	r.forceInitialReconcile(r.Log, "Enforce reconciliation to create a default"+
		"HNCConfiguration singleton if it does not exist")
	return nil
}
