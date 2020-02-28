package reconcilers

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// The ex map is used by reconcilers to exclude namespaces to reconcile. We explicitly
// exclude some default namespaces with constantly changing objects.
// TODO make the exclusion configurable - https://github.com/kubernetes-sigs/multi-tenancy/issues/374
var ex = map[string]bool{
	"kube-system":  true,
	"hnc-system":   true,
	"cert-manager": true,
}

// Create creates all reconcilers.
//
// This function is called both from main.go as well as from the integ tests.
func Create(mgr ctrl.Manager, f *forest.Forest, maxReconciles int, enableHNSReconciler bool) error {
	hcChan := make(chan event.GenericEvent)

	// Create different reconcilers based on if the enableHNSReconciler flag is set or not.
	if enableHNSReconciler {
		// Create the HierarchyConfigReconciler with HNSReconciler enabled.
		hr := &HierarchyConfigReconciler{
			Client:               mgr.GetClient(),
			Log:                  ctrl.Log.WithName("reconcilers").WithName("Hierarchy"),
			Forest:               f,
			Affected:             hcChan,
			HNSReconcilerEnabled: true,
		}
		if err := hr.SetupWithManager(mgr, maxReconciles); err != nil {
			return fmt.Errorf("cannot create Hierarchy reconciler: %s", err.Error())
		}

		// Create HierarchicalNamespaceReconciler.
		hnsr := &HierarchicalNamespaceReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("reconcilers").WithName("HierarchicalNamespace"),
		}
		if err := hnsr.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("cannot create HierarchicalNamespace reconciler: %s", err.Error())
		}
	} else {
		// Create the HierarchyConfigReconciler with HNSReconciler disabled.
		hr := &HierarchyConfigReconciler{
			Client:               mgr.GetClient(),
			Log:                  ctrl.Log.WithName("reconcilers").WithName("Hierarchy"),
			Forest:               f,
			Affected:             hcChan,
			HNSReconcilerEnabled: false,
		}
		if err := hr.SetupWithManager(mgr, maxReconciles); err != nil {
			return fmt.Errorf("cannot create Hierarchy reconciler: %s", err.Error())
		}
	}

	// Create the ConfigReconciler.
	cr := &ConfigReconciler{
		Client:                 mgr.GetClient(),
		Log:                    ctrl.Log.WithName("reconcilers").WithName("HNCConfiguration"),
		Manager:                mgr,
		Forest:                 f,
		Igniter:                make(chan event.GenericEvent),
		HierarchyConfigUpdates: hcChan,
	}
	if err := cr.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot create Config reconciler: %s", err.Error())
	}

	return nil
}
