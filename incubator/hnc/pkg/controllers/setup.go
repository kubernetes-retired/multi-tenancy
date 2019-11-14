package controllers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// Create creates all reconcilers.
//
// This function is called both from main.go as well as from the integ tests.
func Create(mgr ctrl.Manager, f *forest.Forest, maxReconciles int, newObjectController bool) error {
	// Create all object reconcillers
	objReconcilers := []NamespaceSyncer{}
	for _, gvk := range config.GVKs {
		or, err := createObjectReconciler(newObjectController, mgr, f, gvk)
		if err != nil {
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
		Affected: make(chan event.GenericEvent),
	}
	if err := hr.SetupWithManager(mgr, maxReconciles); err != nil {
		return fmt.Errorf("cannot create Hierarchy controller: %s", err.Error())
	}

	return nil
}

func createObjectReconciler(newObjectController bool, mgr ctrl.Manager, f *forest.Forest, gvk schema.GroupVersionKind) (NamespaceSyncer, error) {
	if newObjectController {
		or := &ObjectReconcilerNew{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName(gvk.Kind),
			Forest: f,
			GVK:    gvk,
		}
		return or, or.SetupWithManager(mgr)
	}

	or := &ObjectReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName(gvk.Kind),
		Forest: f,
		GVK:    gvk,
	}
	return or, or.SetupWithManager(mgr)
}
