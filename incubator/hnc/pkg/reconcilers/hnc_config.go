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
		r.Log.Error(err, "Singleton name validation failed.")
		return ctrl.Result{}, nil
	}

	inst, err := r.getSingleton(ctx)
	if err != nil {
		r.Log.Error(err, "Couldn't read singleton.")
		return ctrl.Result{}, err
	}

	// TODO: Modify this and other reconcilers (e.g., hierarchy and object reconcilers) to
	// achieve the reconciliation.
	r.Log.Info("Reconciling cluster-wide HNC configuration.")

	// Clear the existing conditions because we will reconstruct the latest conditions.
	inst.Status.Conditions = nil

	// Create corresponding ObjectReconcilers for newly added types, if needed.
	// TODO: Rename the method syncObjectReconcilers because we might need more than creating ObjectReconcilers in future.
	// For example, we might need to delete an ObjectReconciler if its corresponding type is deleted in the HNCConfiguration.
	r.createObjectReconcilers(inst)

	// Write back to the apiserver.
	// TODO: Update HNCConfiguration.Status before writing the singleton back to the apiserver.
	if err := r.writeSingleton(ctx, inst); err != nil {
		r.Log.Error(err, "Couldn't write singleton.")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
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

// createObjectReconcilers creates corresponding ObjectReconcilers for the newly added types in the
// HNC configuration, if there is any. It also informs the in-memory forest about the newly created
// ObjectReconcilers. If a type is misconfigured, the corresponding object reconciler will not be created.
func (r *ConfigReconciler) createObjectReconcilers(inst *api.HNCConfiguration) {
	// This method is guarded by the forest mutex. The mutex is guarding two actions: creating ObjectReconcilers for
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
	r.Forest.Lock()
	defer r.Forest.Unlock()

	for _, t := range inst.Spec.Types {
		gvk := schema.FromAPIVersionAndKind(t.APIVersion, t.Kind)
		if r.Forest.HasTypeSyncer(gvk) {
			continue
		}
		r.createObjectReconciler(gvk, inst)
	}
}

// createObjectReconciler creates an ObjectReconciler for the given GVK and informs forest about the reconciler.
// TODO: May need to pass in spec instead to provide information about Mode to the ObjectReconciler in future.
func (r *ConfigReconciler) createObjectReconciler(gvk schema.GroupVersionKind, inst *api.HNCConfiguration) {
	r.Log.Info("Creating an object reconciler.", "GVK", gvk)

	or := &ObjectReconciler{
		Client:            r.Client,
		Log:               ctrl.Log.WithName("reconcilers").WithName(gvk.Kind),
		Forest:            r.Forest,
		GVK:               gvk,
		Affected:          make(chan event.GenericEvent),
		AffectedNamespace: r.HierarchyConfigUpdates,
	}

	// TODO: figure out MaxConcurrentReconciles option - https://github.com/kubernetes-sigs/multi-tenancy/issues/291
	if err := or.SetupWithManager(r.Manager, 10); err != nil {
		r.Log.Error(err, "Error while trying to create ObjectReconciler", "gvk", gvk)
		condition := api.HNCConfigurationCondition{
			Code: api.ObjectReconcilerCreationFailed,
			Msg:  fmt.Sprintf("Couldn't create ObjectReconciler for type %s: %s", gvk, err),
		}
		inst.Status.Conditions = append(inst.Status.Conditions, condition)
		return
	}

	// Informs the in-memory forest about the new reconciler by adding it to the types list.
	r.Forest.AddTypeSyncer(or)
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

	msg := "Singleton name is wrong. It should be 'config'."
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
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&api.HNCConfiguration{}).
		Watches(&source.Channel{Source: r.Igniter}, &handler.EnqueueRequestForObject{}).
		Complete(r); err != nil {
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
		"HNCConfiguration singleton if it does not exist.")
	return nil
}
