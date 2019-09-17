package controllers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// Create creates all reconcilers. For now, it also hardcodes the list of GVKs handled by the HNC -
// namely, Secrets, Roles and RoleBindings - but in the future we should get this from a
// configuration object.
//
// This function is called both from main.go as well as from the integ tests.
func Create(mgr ctrl.Manager) error {
	f := forest.NewForest()

	// Create all object reconcillers
	gvks := []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	}
	objReconcilers := []NamespaceSyncer{}
	for _, gvk := range gvks {
		or := &ObjectReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName(gvk.Kind),
			Forest: f,
			GVK:    gvk,
		}
		if err := or.SetupWithManager(mgr); err != nil {
			return fmt.Errorf("cannot create %v controller: %s", gvk, err.Error())
		}
		objReconcilers = append(objReconcilers, or)
	}

	// Create the HierarchyReconciler, passing it the object reconcillers so it can call them.
	hr := &HierarchyReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Hierarchy"),
		Forest: f,
		Types:  objReconcilers,
	}
	if err := hr.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot create Hierarchy controller: %s", err.Error())
	}

	// Finally, create the Namespace Reconciler, passing it the Hierarchy Reconciler so it can be
	// called when namespaces are created or deleted.
	if err := (&NamespaceReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Namespace"),
		Syncer: hr,
	}).SetupWithManager(mgr); err != nil {
		return fmt.Errorf("cannot create Namespace controller: %s", err.Error())
	}

	return nil
}
