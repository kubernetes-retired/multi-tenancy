package reconcilers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

// RemoveObsoleteCRDVersionReconciler tries to remove obsolete version from CRD.
type RemoveObsoleteCRDVersionReconciler struct {
	Client *apiextension.Clientset
	Log    logr.Logger

	// ObsoleteVersion is what we want to remove from the CRD status.storedVersions.
	ObsoleteVersion string

	// CRDNames is a set of the CRD names that we want to remove the obsolete
	// version from its status.storedVersions.
	CRDNames nameSet
}

// nameSet keeps track of a group of unique CRD names.
type nameSet map[string]bool

// Reconcile is the entry point to the reconciler.
func (r *RemoveObsoleteCRDVersionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("crd", req.Name, "oldVersion", r.ObsoleteVersion)

	// Early exit if the CRD is not the ones we want to remove the version.
	if !r.CRDNames[req.Name] {
		return ctrl.Result{}, nil
	}

	nm := req.Name
	log.Info("Reconciling")

	// Get CRD.
	inst, err := r.Client.ApiextensionsV1beta1().CustomResourceDefinitions().Get(ctx, nm, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// The CRD is deleted
			log.Info("The CRD is deleted. No action is needed.")
			return ctrl.Result{}, nil
		}
		log.Error(err, "couldn't read CRD")
		return ctrl.Result{}, err
	}

	// Update CRD status to remove the obsolete version.
	if err := r.removeVersion(ctx, log, inst); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RemoveObsoleteCRDVersionReconciler) removeVersion(ctx context.Context, log logr.Logger, inst *apiextensions.CustomResourceDefinition) error {
	vs := []string{}
	found := false
	for _, v := range inst.Status.StoredVersions {
		if v == r.ObsoleteVersion {
			found = true
			continue
		}
		vs = append(vs, v)
	}
	if !found {
		log.Info("The old version is not found. No action is needed.")
		return nil
	}
	inst.Status.StoredVersions = vs
	return r.updateCRDStatus(ctx, log, inst)
}

func (r *RemoveObsoleteCRDVersionReconciler) updateCRDStatus(ctx context.Context, log logr.Logger, inst *apiextensions.CustomResourceDefinition) error {
	msg := fmt.Sprintf("Write CRD status.storedVersions: %v", inst.Status.StoredVersions)
	log.Info(msg)
	inst, err := r.Client.ApiextensionsV1beta1().CustomResourceDefinitions().UpdateStatus(ctx, inst, v1.UpdateOptions{})
	if err != nil {
		log.Error(err, "while updating apiserver")
		return err
	}
	return nil
}

// SetupWithManager builds a controller with the reconciler.
func (r *RemoveObsoleteCRDVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Register the reconciler
	err := ctrl.NewControllerManagedBy(mgr).
		For(&apiextensions.CustomResourceDefinition{}).
		Complete(r)
	if err != nil {
		return err
	}

	return nil
}
