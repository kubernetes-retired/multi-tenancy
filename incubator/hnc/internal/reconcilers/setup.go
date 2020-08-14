package reconcilers

import (
	"context"
	"fmt"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextension "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
)

var crds = map[string]bool{
	"hncconfigurations.hnc.x-k8s.io":       true,
	"subnamespaceanchors.hnc.x-k8s.io":     true,
	"hierarchyconfigurations.hnc.x-k8s.io": true,
}

// Create creates all reconcilers.
//
// This function is called both from main.go as well as from the integ tests.
func Create(mgr ctrl.Manager, f *forest.Forest, maxReconciles int, removeOldCRDVersion bool) error {
	hcChan := make(chan event.GenericEvent)
	anchorChan := make(chan event.GenericEvent)

	// Create AnchorReconciler.
	sar := &AnchorReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("reconcilers").WithName("Anchor"),
		forest:   f,
		Affected: anchorChan,
	}
	if err := sar.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot create anchor reconciler: %s", err.Error())
	}

	// Create the HierarchyConfigReconciler with a pointer to the Anchor reconciler.
	hcr := &HierarchyConfigReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("reconcilers").WithName("Hierarchy"),
		Forest:   f,
		sar:      sar,
		Affected: hcChan,
	}
	if err := hcr.SetupWithManager(mgr, maxReconciles); err != nil {
		return fmt.Errorf("cannot create Hierarchy reconciler: %s", err.Error())
	}

	// Create the ConfigReconciler.
	hnccrSingleton = &ConfigReconciler{
		Client:                 mgr.GetClient(),
		Log:                    ctrl.Log.WithName("reconcilers").WithName("HNCConfiguration"),
		Manager:                mgr,
		Forest:                 f,
		Trigger:                make(chan event.GenericEvent),
		HierarchyConfigUpdates: hcChan,
	}
	if err := hnccrSingleton.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot create Config reconciler: %s", err.Error())
	}

	if removeOldCRDVersion {
		if err := createRemoveCRDVersionReconciler(mgr, "v1alpha1"); err != nil {
			return err
		}
	}

	return nil
}

func createRemoveCRDVersionReconciler(mgr ctrl.Manager, v string) error {
	// Create a client to update CRD status sub-resource to remove a version from
	// CRD status.storedVersions. Simply updating the CRD won't work. See examples
	// and discussions at https://github.com/elastic/cloud-on-k8s/issues/2196.
	client, err := apiextension.NewForConfig(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("cannot create the client for CRDStoredVersions reconcile: %s", err.Error())
	}
	// Create the reconciler.
	crdStoredVersions := &RemoveObsoleteCRDVersionReconciler{
		Client:          client,
		Log:             ctrl.Log.WithName("reconcilers").WithName("CRDStoredVersions"),
		ObsoleteVersion: v,
		CRDNames:        crds,
	}
	if err := crdStoredVersions.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot create CRDStoredVersions reconciler: %s", err.Error())
	}
	return nil
}

// crdClient is any client that can be used in isDeletingCRD (i.e. any reconciler).
type crdClient interface {
	Get(context.Context, types.NamespacedName, runtime.Object) error
}

// isDeletingCRD returns true if the specified HNC CRD is being or has been deleted. The argument
// expected is the CRD name minus the HNC suffix, e.g. "hierarchyconfigurations".
func isDeletingCRD(ctx context.Context, c crdClient, nm string) (bool, error) {
	crd := &apiextensions.CustomResourceDefinition{}
	nsn := types.NamespacedName{Name: nm + "." + api.MetaGroup}
	if err := c.Get(ctx, nsn, crd); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return !crd.DeletionTimestamp.IsZero(), nil
}
