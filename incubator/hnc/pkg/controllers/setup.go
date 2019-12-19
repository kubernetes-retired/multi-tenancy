package controllers

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// Counters of total/current number of hier/object reconciliation attempts.
var (
	hcTot int32
	hcCur int32
	obTot int32
	obCur int32
)

// The ex map is only used by reconcile counters for performance testing. We explicitly
// exclude some default namespaces with constantly changing objects.
var ex = map[string]bool{
	"kube-system": true,
	"hnc-system":  true,
}

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
			Client:   mgr.GetClient(),
			Log:      ctrl.Log.WithName("controllers").WithName(gvk.Kind),
			Forest:   f,
			GVK:      gvk,
			Affected: make(chan event.GenericEvent),
		}
		// TODO figure out MaxConcurrentReconciles option - https://github.com/kubernetes-sigs/multi-tenancy/issues/291
		return or, or.SetupWithManager(mgr, 10)
	}

	or := &ObjectReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName(gvk.Kind),
		Forest: f,
		GVK:    gvk,
	}
	return or, or.SetupWithManager(mgr)
}

// LogActivity generates logs for performance testing.
func LogActivity() {
	log := ctrl.Log.WithName("reconcileCounter")
	var total, cur int32 = 0, 0
	go func() {
		// run forever
		for {
			// Log activity only when the controllers were still working in the last 0.5s.
			time.Sleep(500 * time.Millisecond)
			if hcTot+obTot != total || cur != 0 {
				log.Info("Activity", "hcTot", hcTot, "hcCur", hcCur, "obTot", obTot, "obCur", obCur)
			}
			total = hcTot + obTot
			cur = hcCur + obCur
		}
	}()
}
