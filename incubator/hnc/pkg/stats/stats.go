package stats

import (
	"math"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
)

type object struct {
	totalReconciles counter
	curReconciles   counter
	apiWrites       counter
}

type objects map[schema.GroupKind]*object

type stat struct {
	// actionID is the number of controller actions devided by idles
	actionID counter

	// totalHierConfigReconciles is the total number of HierarchyConfig reconciliations.
	totalHierConfigReconciles counter

	// curHierConfigReconciles is the currently undergoing number of HierarchyConfig reconciliations.
	curHierConfigReconciles counter

	// hierConfigWrites is the total number of HierarchyConfig writes.
	hierConfigWrites counter

	// namespaceWrites is the total number of Namespace writes.
	namespaceWrites counter

	objects objects
}

var stats stat

// StartHierConfigReconcile updates stats when hierarchyConfig
// reconciliation starts.
func StartHierConfigReconcile() {
	peak.lock.Lock()
	defer peak.lock.Unlock()
	stats.totalHierConfigReconciles.incr()
	stats.curHierConfigReconciles.incr()

	recordMetric(stats.totalHierConfigReconciles, hierConfigReconcileTotal)
	// Only update the maxConcurrent value in StartReconcile() since it's impossible to
	// get the max from StopReconcile() when the reconcile number decrements.
	peak.concurrentHierConfigReconcile = max(peak.concurrentHierConfigReconcile, stats.curHierConfigReconciles)
}

// StopHierConfigReconcile updates stats when hierarchyConfig
// reconciliation finishes.
func StopHierConfigReconcile() {
	stats.curHierConfigReconciles.decr()
}

// StartObjReconcile updates the stats for objects with common GK
// when an object reconciliation starts.
func StartObjReconcile(gvk schema.GroupVersionKind) {
	peak.lock.Lock()
	defer peak.lock.Unlock()
	gk := gvk.GroupKind()
	if _, ok := stats.objects[gk]; !ok {
		stats.objects[gk] = &object{}
	}
	stats.objects[gk].totalReconciles.incr()
	stats.objects[gk].curReconciles.incr()

	recordTagMetric(stats.objects[gk].totalReconciles, objectReconcileTotal, KeyGroupKind, gk.String())
	// Only update the maxConcurrent value in StartReconcile() since it's impossible to
	// get the max from StopReconcile() when the reconcile number decrements.
	peak.concurrentObjectReconcile[gk] = max(peak.concurrentObjectReconcile[gk], stats.objects[gk].curReconciles)
}

// StopObjReconcile updates the stats for objects with common GK
// when an object reconciliation finishes.
func StopObjReconcile(gvk schema.GroupVersionKind) {
	gk := gvk.GroupKind()
	stats.objects[gk].curReconciles.decr()
}

// WriteNamespace updates stats when writing namespace instance.
func WriteNamespace() {
	stats.namespaceWrites.incr()

	recordMetric(stats.namespaceWrites, namespaceWritesTotal)
}

// WriteHierConfig updates stats when writing hierarchyConfig instance.
func WriteHierConfig() {
	stats.hierConfigWrites.incr()

	recordMetric(stats.hierConfigWrites, hierConfigWritesTotal)
}

// WriteObject updates the object stats by GK when writing the object.
func WriteObject(gvk schema.GroupVersionKind) {
	gk := gvk.GroupKind()
	stats.objects[gk].apiWrites.incr()

	recordTagMetric(stats.objects[gk].apiWrites, objectWritesTotal, KeyGroupKind, gk.String())
}

func init() {
	objects := make(map[schema.GroupKind]*object)
	peak = periodicPeak{
		lock:                          sync.Mutex{},
		concurrentHierConfigReconcile: 0,
		concurrentObjectReconcile:     make(map[schema.GroupKind]counter),
	}
	stats = stat{
		actionID: 1,
		objects:  objects,
	}
	startRecordingMetrics()
}

// StartLoggingActivity generates logs for performance testing.
func StartLoggingActivity() {
	log := ctrl.Log.WithName("reconcileCounter")
	var total, lastTotal, lastCur counter = 0, 0, 0
	working := false
	go logging(log, total, lastTotal, lastCur, working)
}

func logging(log logr.Logger, total, lastTotal, lastCur counter, working bool) {
	// run forever
	for {
		// Log activity only when the controllers were still working in the last 0.5s.
		time.Sleep(500 * time.Millisecond)
		total = stats.totalHierConfigReconciles + getTotalObjReconciles()
		// If lastCur is not 0 yet, still generate a log for the past 0.5s.
		if total != lastTotal || lastCur != 0 {
			// If the controller was previously idle, change its status and log it's started.
			if working == false {
				working = true
				logActivity(log, "start")
			} else {
				logActivity(log, "continue")
			}
		} else {
			// If the controller was previously working, change its status and log it's finished.
			if working == true {
				working = false
				logActivity(log, "finish")
				stats.actionID++
			}
		}
		lastTotal = total
		lastCur = stats.curHierConfigReconciles + getCurObjReconciles()
	}
}

func logActivity(log logr.Logger, status string) {
	log.Info("Activity",
		"Action", stats.actionID,
		"Status", status,
		"HierConfigWrites", stats.hierConfigWrites,
		"NamespaceWrites", stats.namespaceWrites,
		"ObjectWrites", getObjWrites(),
		"TotalHierConfigReconciles", stats.totalHierConfigReconciles,
		"CurHierConfigReconciles", stats.curHierConfigReconciles,
		"TotalObjReconciles", getTotalObjReconciles(),
		"CurObjReconciles", getCurObjReconciles())
}

func getTotalObjReconciles() counter {
	var total counter
	for _, obj := range stats.objects {
		total += obj.totalReconciles
	}
	return total
}

func getCurObjReconciles() counter {
	var cur counter
	for _, obj := range stats.objects {
		cur += obj.curReconciles
	}
	return cur
}

func getObjWrites() counter {
	var writes counter
	for _, obj := range stats.objects {
		writes += obj.apiWrites
	}
	return writes
}

func max(a counter, b counter) counter {
	return counter(math.Max(float64(a), float64(b)))
}
