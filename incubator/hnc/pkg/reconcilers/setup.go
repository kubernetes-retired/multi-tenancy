package reconcilers

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// The EX map is used by reconcilers to exclude namespaces to reconcile. We explicitly
// exclude some default namespaces with constantly changing objects.
// TODO make the exclusion configurable - https://github.com/kubernetes-sigs/multi-tenancy/issues/374
var EX = map[string]bool{
	"kube-system":  true,
	"kube-public":  true,
	"hnc-system":   true,
	"cert-manager": true,
}

// Create creates all reconcilers.
//
// This function is called both from main.go as well as from the integ tests.
func Create(mgr ctrl.Manager, f *forest.Forest, maxReconciles int) error {
	hcChan := make(chan event.GenericEvent)
	hnsChan := make(chan event.GenericEvent)

	// Create the HierarchyConfigReconciler with HNSReconciler enabled.
	hcr := &HierarchyConfigReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("reconcilers").WithName("Hierarchy"),
		Forest:   f,
		Affected: hcChan,
	}

	// Create HierarchicalNamespaceReconciler.
	hnsr := &HierarchicalNamespaceReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("reconcilers").WithName("HierarchicalNamespace"),
		forest:   f,
		hcr:      hcr,
		Affected: hnsChan,
	}

	// Set hcr.hnsr after the HNS reconciler is created.
	hcr.hnsr = hnsr
	if err := hcr.SetupWithManager(mgr, maxReconciles); err != nil {
		return fmt.Errorf("cannot create Hierarchy reconciler: %s", err.Error())
	}
	if err := hnsr.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot create HierarchicalNamespace reconciler: %s", err.Error())
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
