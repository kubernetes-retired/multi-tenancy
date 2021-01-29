package reconcilers

import (
	"context"
	"fmt"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/event"

	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
	"sigs.k8s.io/multi-tenancy/incubator/hnc/internal/forest"
)

// FakeDeleteCRDClient is a "fake" client used for testing only
type FakeDeleteCRDClient struct{}

// FakeDeleteCRDClient doesn't return any err on Get() because none of the reconciler test performs CRD deletion
func (f FakeDeleteCRDClient) Get(context.Context, types.NamespacedName, client.Object) error {
	return nil
}

var crds = map[string]bool{
	"hncconfigurations.hnc.x-k8s.io":       true,
	"subnamespaceanchors.hnc.x-k8s.io":     true,
	"hierarchyconfigurations.hnc.x-k8s.io": true,
}

// deleteCRDClientType could be either a real client or FakeDeleteCRDClient
type deleteCRDClientType interface {
	Get(context.Context, types.NamespacedName, client.Object) error
}

// deleteCRDClient is an uncached client for checking CRD deletion
var deleteCRDClient deleteCRDClientType

// Create creates all reconcilers.
//
// This function is called both from main.go as well as from the integ tests.
func Create(mgr ctrl.Manager, f *forest.Forest, maxReconciles int, useFakeClient bool) error {
	hcChan := make(chan event.GenericEvent)
	anchorChan := make(chan event.GenericEvent)

	// Create uncached client for CRD deletion check
	if !useFakeClient {
		var err error
		deleteCRDClient, err = client.New(config.GetConfigOrDie(), client.Options{
			Scheme: mgr.GetScheme(),
			// I'm not sure if this mapper is needed - @ginnyji Dec2020
			Mapper: mgr.GetRESTMapper(),
		})
		if err != nil {
			return fmt.Errorf("cannot create deleteCRDClient: %s", err.Error())
		}
	} else {
		deleteCRDClient = FakeDeleteCRDClient{}
	}
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

	return nil
}

// crdClient is any client that can be used in isDeletingCRD (i.e. any reconciler).
type crdClient interface {
	Get(context.Context, types.NamespacedName, client.Object) error
}

// isDeletingCRD returns true if the specified HNC CRD is being or has been deleted. The argument
// expected is the CRD name minus the HNC suffix, e.g. "hierarchyconfigurations".
func isDeletingCRD(ctx context.Context, nm string) (bool, error) {
	crd := &apiextensions.CustomResourceDefinition{}
	nsn := types.NamespacedName{Name: nm + "." + api.MetaGroup}
	if err := deleteCRDClient.Get(ctx, nsn, crd); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return !crd.DeletionTimestamp.IsZero(), nil
}
