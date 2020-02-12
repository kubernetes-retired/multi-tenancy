package controllers

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
)

// ConfigReconciler is responsible for determining the HNC configuration from the HNCConfiguration CR,
// as well as ensuring all objects are propagated correctly when the HNC configuration changes.
// It can also set the status of the HNCConfiguration CR.
type ConfigReconciler struct {
	client.Client
	Log logr.Logger
}

// Reconcile sets up some basic variable and logs the Spec.
// TODO: Updates the comment above when adding more logic to the Reconcile method.
func (r *ConfigReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	// TODO: Surface the error more prominently and add a validating admission controller to prevent
	// the problem in the first place.
	if req.NamespacedName.Name != api.HNCConfigSingleton {
		r.Log.Error(nil, "Singleton name is wrong. It should be 'config'.")
		return ctrl.Result{}, nil
	}

	ctx := context.Background()
	inst, err := r.getSingleton(ctx)
	if err != nil {
		r.Log.Error(err, "Couldn't read singleton.")
		return ctrl.Result{}, nil
	}

	// TODO: Modify this and other reconcilers (e.g., hierarchy and object reconcilers) to
	// achieve the reconciliation.
	r.Log.Info("Reconciling cluster-wide HNC configuration.")

	// Write back to the apiserver.
	if err := r.writeSingleton(ctx, inst); err != nil {
		r.Log.Error(err, "Couldn't write singleton.")
		return ctrl.Result{}, err
	}

	r.logSpec(inst)
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

// logSpec logs current Spec of the CRD.
// TODO: This method is mainly for debuging and testing in the early development stage. Remove
// this method when the implementation is compeleted.
func (r *ConfigReconciler) logSpec(inst *api.HNCConfiguration) {
	r.Log.Info("Record length of Types", "length", len(inst.Spec.Types))
	for _, t := range inst.Spec.Types {
		r.Log.Info("spec:", "apiVersion: ", t.APIVersion)
		r.Log.Info("spec:", "kind: ", t.Kind)
		r.Log.Info("spec:", "mode: ", t.Mode)
	}
}

// SetupWithManager builds a controller with the reconciler.
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.HNCConfiguration{}).
		Complete(r)
}
