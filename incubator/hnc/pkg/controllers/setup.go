package controllers

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// The ex map is used by controllers to exclude namespaces to reconcile. We explicitly
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
func Create(mgr ctrl.Manager, f *forest.Forest, maxReconciles int) error {
	hcChan := make(chan event.GenericEvent)

	// Create all object reconcillers
	objReconcilers := []NamespaceSyncer{}
	for _, gvk := range config.GVKs {
		or := &ObjectReconciler{
			Client:            mgr.GetClient(),
			Log:               ctrl.Log.WithName("controllers").WithName(gvk.Kind),
			Forest:            f,
			GVK:               gvk,
			Affected:          make(chan event.GenericEvent),
			AffectedNamespace: hcChan,
		}
		// TODO figure out MaxConcurrentReconciles option - https://github.com/kubernetes-sigs/multi-tenancy/issues/291
		if err := or.SetupWithManager(mgr, 10); err != nil {
			return fmt.Errorf("cannot create %v controller: %s", gvk, err.Error())
		}
		objReconcilers = append(objReconcilers, or)
	}

	// Create the HierarchyReconciler, passing it the object reconcillers so it can call them.
	hr := &HierarchyReconciler{
		Client:   mgr.GetClient(),
		Log:      ctrl.Log.WithName("controllers").WithName("Hierarchy"),
		Forest:   f,
		Types:    objReconcilers,
		Affected: hcChan,
	}
	if err := hr.SetupWithManager(mgr, maxReconciles); err != nil {
		return fmt.Errorf("cannot create Hierarchy controller: %s", err.Error())
	}

	// Create the ConfigReconciler.
	cr := &ConfigReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("HNCConfiguration"),
	}
	if err := cr.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot create Config controller: %s", err.Error())
	}

	return nil
}
