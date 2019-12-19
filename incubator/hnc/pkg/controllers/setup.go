package controllers

import (
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/config"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/pkg/forest"
)

// Counters of total/current number of hier/object reconciliation attempts.
var (
	hcTot     int32
	hcCur     int32
	obTot     int32
	obCur     int32
	mutexQ    int32
	apiCall   int32
	hcWriteHC int32
	hcWriteNS int32
	hcRead    int32
	objWrite  int32
	objRead   int32
)

// The ex map is used by controllers to exclude namespaces to reconcile. We explicitly
// exclude some default namespaces with constantly changing objects.
var ex = map[string]bool{
	"kube-system": true,
	"hnc-system":  true,
}

// Create creates all reconcilers.
//
// This function is called both from main.go as well as from the integ tests.
func Create(mgr ctrl.Manager, f *forest.Forest, maxReconciles int, newObjectController bool) error {
	hcChan := make(chan event.GenericEvent)

	// Create all object reconcillers
	objReconcilers := []NamespaceSyncer{}
	for _, gvk := range config.GVKs {
		or, err := createObjectReconciler(newObjectController, mgr, f, gvk, hcChan)
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
		Affected: hcChan,
	}
	if err := hr.SetupWithManager(mgr, maxReconciles); err != nil {
		return fmt.Errorf("cannot create Hierarchy controller: %s", err.Error())
	}

	return nil
}

func createObjectReconciler(newObjectController bool, mgr ctrl.Manager, f *forest.Forest, gvk schema.GroupVersionKind, hcChan chan event.GenericEvent) (NamespaceSyncer, error) {
	if newObjectController {
		or := &ObjectReconcilerNew{
			Client:            mgr.GetClient(),
			Log:               ctrl.Log.WithName("controllers").WithName(gvk.Kind),
			Forest:            f,
			GVK:               gvk,
			Affected:          make(chan event.GenericEvent),
			AffectedNamespace: hcChan,
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
	working := false
	actN := 1
	go func() {
		// run forever
		for {
			// Log activity only when the controllers were still working in the last 0.5s.
			time.Sleep(500 * time.Millisecond)
			if hcTot+obTot != total || cur != 0 {
				// If the controller was previously idle, change its status and log it's started.
				if working == false {
					working = true
					log.Info("Activity-"+strconv.Itoa(actN)+"-started", "hcNSwrite", hcWriteNS, "hcWrite", hcWriteHC, "hcRead", hcRead, "objWrite", objWrite, "objRead", objRead, "hcTot", hcTot, "obTot", obTot)
				}
				log.Info("Activity", "hcNSwrite", hcWriteNS, "hcWrite", hcWriteHC, "hcRead", hcRead, "objWrite", objWrite, "objRead", objRead, "apiCall", apiCall, "mutexQ", mutexQ, "hcTot", hcTot, "hcCur", hcCur, "obTot", obTot, "obCur", obCur)
			} else {
				// If the controller was previously working, change its status and log it's finished.
				if working == true {
					working = false
					log.Info("Activity-"+strconv.Itoa(actN)+"-finished", "hcNSwrite", hcWriteNS, "hcWrite", hcWriteHC, "hcRead", hcRead, "objWrite", objWrite, "objRead", objRead, "hcTot", hcTot, "obTot", obTot)
					actN++
				}
			}
			total = hcTot + obTot
			cur = hcCur + obCur
		}
	}()
}
