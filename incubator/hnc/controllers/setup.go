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
func Create(mgr ctrl.Manager, labelOnly bool) error {
	f := forest.NewForest()

	// Create all object reconcillers
	gvks := []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
		{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	}
	objReconcilers := []TypeReconciler{}
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

	if !labelOnly {
		// Create the HierarchyReconciler, passing it the object reconcillers so it can call them.
		if err := (&HierarchyReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("Hierarchy"),
			Forest: f,
			Types:  objReconcilers,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("cannot create Hierarchy controller: %s", err.Error())
		}

		// The NamespaceReconciler can be created in any order.
		if err := (&NamespaceReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("Namespace"),
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("cannot create Namespace controller: %s", err.Error())
		}
	} else {
		// Create the namespace label reconciler
		if err := (&LabelReconciler{
			Client: mgr.GetClient(),
			Log:    ctrl.Log.WithName("controllers").WithName("Namespace"),
			Forest: f,
			Types:  objReconcilers,
		}).SetupWithManager(mgr); err != nil {
			return fmt.Errorf("cannot create label controller: %s", err.Error())
		}
	}

	return nil
}
