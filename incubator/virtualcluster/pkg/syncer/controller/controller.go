/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgocache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/handler"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

// MultiClusterController implements the controller pattern.
// A MultiClusterController owns a client-go workqueue. Watch methods set up the queue to receive reconcile requests,
// e.g., on resource CRUD events in a cluster. Then the Requests are processed by the user-provided Reconciler.
// A MultiClusterController can watch multiple resources in multiple clusters. It saves those clusters in a set,
// so the ControllerManager knows which caches to start and sync before starting the Controller.
type MultiClusterController struct {
	// name is used to uniquely identify a Controller in tracing, logging and monitoring.  Name is required.
	name string

	// objectType is the type of object to watch.  e.g. &v1.Pod{}
	objectType runtime.Object

	// clusters is the internal cluster set this controller watches.
	clusters map[Cluster]struct{}

	Options
}

// Options are the arguments for creating a new Controller.
type Options struct {
	// JitterPeriod is the time to wait after an error to start working again.
	JitterPeriod time.Duration

	// MaxConcurrentReconciles is the number of concurrent control loops.
	// Use this if your Reconciler is slow, but thread safe.
	MaxConcurrentReconciles int

	// Reconciler is a function that can be called at any time with the Name / Namespace of an object and
	// ensures that the state of the system matches the state specified in the object.
	// Defaults to the DefaultReconcileFunc.
	Reconciler reconciler.Reconciler

	// Queue can be used to override the default queue.
	Queue workqueue.RateLimitingInterface
}

// Cache is the interface used by Controller to start and wait for caches to sync.
type Cache interface {
	Start(stop <-chan struct{}) error
	WaitForCacheSync(stop <-chan struct{}) bool
}

// Cluster decouples the controller package from the cluster package.
type Cluster interface {
	GetClusterName() string
	AddEventHandler(runtime.Object, clientgocache.ResourceEventHandler) error
	GetCache() (cache.Cache, error)
	GetClientInfo() *reconciler.ClusterInfo
	Cache
}

// NewController creates a new Controller.
func NewController(name string, objectType runtime.Object, options Options) (*MultiClusterController, error) {
	if options.Reconciler == nil {
		return nil, fmt.Errorf("must specify Reconciler")
	}

	if len(name) == 0 {
		return nil, fmt.Errorf("must specify Name for Controller")
	}

	c := &MultiClusterController{
		name:       name,
		objectType: objectType,
		clusters:   make(map[Cluster]struct{}),
		Options:    options,
	}

	if c.JitterPeriod == 0 {
		c.JitterPeriod = 1 * time.Second
	}

	if c.MaxConcurrentReconciles <= 0 {
		c.MaxConcurrentReconciles = 1
	}

	if c.Queue == nil {
		c.Queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	}

	return c, nil
}

// WatchOptions is used as an argument of WatchResource methods (just a placeholder for now).
// TODO: consider implementing predicates.
type WatchOptions struct {
}

// WatchClusterResource configures the Controller to watch resources of the same Kind as objectType,
// in the specified cluster, generating reconcile Requests from the Cluster's context
// and the watched objects' namespaces and names.
func (c *MultiClusterController) WatchClusterResource(cluster Cluster, o WatchOptions) error {
	c.clusters[cluster] = struct{}{}
	h := &handler.EnqueueRequestForObject{Cluster: cluster.GetClientInfo(), Queue: c.Queue}
	return cluster.AddEventHandler(c.objectType, h)
}

// Start starts the ClustersController's control loops (as many as MaxConcurrentReconciles) in separate channels
// and blocks until an empty struct is sent to the stop channel.
func (c *MultiClusterController) Start(stop <-chan struct{}) error {
	// pre start all the cluster caches
	wg := &sync.WaitGroup{}
	wg.Add(len(c.clusters))

	errCh := make(chan error)
	for cl := range c.clusters {
		go func(cl Cluster) {
			if err := cl.Start(stop); err != nil {
				errCh <- err
			}
		}(cl)

		go func(cl Cluster) {
			defer wg.Done()

			if ok := cl.WaitForCacheSync(stop); !ok {
				errCh <- fmt.Errorf("failed to wait for caches to sync")
			}
		}(cl)
	}

	wg.Wait()

	klog.Infof("start clusters-controller %q", c.name)

	defer c.Queue.ShutDown()

	for i := 0; i < c.MaxConcurrentReconciles; i++ {
		go wait.Until(c.worker, c.JitterPeriod, stop)
	}

	select {
	case <-stop:
		return nil
	case err := <-errCh:
		return err
	}
}

// Get returns object with specific cluster, namespace and name.
func (c *MultiClusterController) Get(cluster, namespace, name string) (interface{}, error) {
	return nil, nil
}

func (c *MultiClusterController) GetCluster(clusterName string) Cluster {
	for cluster, _ := range c.clusters {
		if cluster.GetClusterName() == clusterName {
			return cluster
		}
	}
	return nil
}

// worker runs a worker thread that just dequeues items, processes them, and marks them done.
// It enforces that the reconcileHandler is never invoked concurrently with the same object.
func (c *MultiClusterController) worker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it.
func (c *MultiClusterController) processNextWorkItem() bool {
	obj, shutdown := c.Queue.Get()
	if obj == nil {
		c.Queue.Forget(obj)
	}

	if shutdown {
		// Stop working
		klog.Warning("Shutting down. Ignore work item and stop working.")
		return false
	}

	// We call Done here so the workqueue knows we have finished
	// processing this item. We also must remember to call Forget if we
	// do not want this work item being re-queued. For example, we do
	// not call Forget if a transient error occurs, instead the item is
	// put back on the workqueue and attempted again after a back-off
	// period.
	defer c.Queue.Done(obj)

	var req reconciler.Request
	var ok bool
	if req, ok = obj.(reconciler.Request); !ok {
		// As the item in the workqueue is actually invalid, we call
		// Forget here else we'd go into a loop of attempting to
		// process a work item that is invalid.
		c.Queue.Forget(obj)
		klog.Warning("Work item is not a Request. Ignore it. Next.")
		// Return true, don't take a break
		return true
	}
	// RunInformersAndControllers the syncHandler, passing it the cluster/namespace/Name
	// string of the resource to be synced.
	if result, err := c.Reconciler.Reconcile(req); err != nil {
		c.Queue.AddRateLimited(req)
		klog.Error(err)
		klog.Error("Could not reconcile Request. Stop working.")
		return false
	} else if result.RequeueAfter > 0 {
		c.Queue.AddAfter(req, result.RequeueAfter)
		return true
	} else if result.Requeue {
		c.Queue.AddRateLimited(req)
		return true
	}

	// Finally, if no error occurs we Forget this item so it does not
	// get queued again until another change happens.
	c.Queue.Forget(obj)

	// Return true, don't take a break
	return true
}
