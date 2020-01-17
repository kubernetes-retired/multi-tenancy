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

package mccontroller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	clientgocache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/cache"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/handler"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	"k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiClusterController implements the multicluster controller pattern.
// A MultiClusterController owns a client-go workqueue. The WatchClusterResource methods set
// up the queue to receive reconcile requests, e.g., CRUD events from a tenant cluster.
// The Requests are processed by the user-provided Reconciler.
// MultiClusterController saves all watched tenant clusters in a set, so the ControllerManager knows
// which caches to start and sync before starting the MultiClusterController.
type MultiClusterController struct {
	sync.Mutex
	// name is used to uniquely identify a Controller in tracing, logging and monitoring.  Name is required.
	name string

	// objectType is the type of object to watch.  e.g. &v1.Pod{}
	objectType runtime.Object

	// clusters is the internal cluster set this controller watches.
	clusters map[string]ClusterInterface

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
	Start() error
	WaitForCacheSync() bool
	Synced() bool
	Stop()
}

// ClusterInterface decouples the controller package from the cluster package.
type ClusterInterface interface {
	GetClusterName() string
	GetSpec() *v1alpha1.VirtualclusterSpec
	AddEventHandler(runtime.Object, clientgocache.ResourceEventHandler) error
	GetCache() (cache.Cache, error)
	GetClientInfo() *reconciler.ClusterInfo
	GetClient() (*clientset.Clientset, error)
	Cache
}

// NewMCController creates a new MultiClusterController.
func NewMCController(name string, objectType runtime.Object, options Options) (*MultiClusterController, error) {
	if options.Reconciler == nil {
		return nil, fmt.Errorf("must specify Reconciler")
	}

	if len(name) == 0 {
		return nil, fmt.Errorf("must specify Name for Controller")
	}

	c := &MultiClusterController{
		name:       name,
		objectType: objectType,
		clusters:   make(map[string]ClusterInterface),
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
// in the specified cluster, generating reconcile Requests from the ClusterInterface's context
// and the watched objects' namespaces and names.
func (c *MultiClusterController) WatchClusterResource(cluster ClusterInterface, o WatchOptions) error {
	c.Lock()
	defer c.Unlock()
	if _, exist := c.clusters[cluster.GetClusterName()]; exist {
		return nil
	}
	c.clusters[cluster.GetClusterName()] = cluster

	if c.objectType == nil {
		return nil
	}

	h := &handler.EnqueueRequestForObject{Cluster: cluster.GetClientInfo(), Queue: c.Queue}
	return cluster.AddEventHandler(c.objectType, h)
}

// TeardownClusterResource forget the cluster it watches.
// The cluster informer should stop together.
func (c *MultiClusterController) TeardownClusterResource(cluster ClusterInterface) {
	c.Lock()
	delete(c.clusters, cluster.GetClusterName())
	c.Unlock()
}

// Start starts the ClustersController's control loops (as many as MaxConcurrentReconciles) in separate channels
// and blocks until an empty struct is sent to the stop channel.
func (c *MultiClusterController) Start(stop <-chan struct{}) error {
	klog.Infof("start clusters-controller %q", c.name)

	defer c.Queue.ShutDown()

	for i := 0; i < c.MaxConcurrentReconciles; i++ {
		go wait.Until(c.worker, c.JitterPeriod, stop)
	}

	select {
	case <-stop:
		return nil
	}
}

// Get returns object with specific cluster, namespace and name.
func (c *MultiClusterController) Get(clusterName, namespace, name string) (interface{}, error) {
	cluster := c.GetCluster(clusterName)
	if cluster == nil {
		return nil, fmt.Errorf("could not find cluster %s", clusterName)
	}
	instance := getTargetObject(c.objectType)
	clusterCache, err := cluster.GetCache()
	if err != nil {
		return nil, err
	}
	err = clusterCache.Get(context.TODO(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, instance)
	return instance, err
}

func (c *MultiClusterController) GetCluster(clusterName string) ClusterInterface {
	c.Lock()
	defer c.Unlock()
	for _, cluster := range c.clusters {
		// If cluster cache has not been synced, we assume it is not available
		if cluster.GetClusterName() == clusterName && cluster.Synced() {
			return cluster
		}
	}
	return nil
}

func (c *MultiClusterController) GetClusterNames() []string {
	c.Lock()
	defer c.Unlock()
	var names []string
	for _, cluster := range c.clusters {
		names = append(names, cluster.GetClusterName())
	}
	return names
}

// RequeueObject requeues the cluster object, thus reconcileHandler can reconcile it again.
func (c *MultiClusterController) RequeueObject(clusterName string, obj interface{}, event reconciler.EventType) {
	o, err := meta.Accessor(obj)
	if err != nil {
		return
	}

	cluster := c.GetCluster(clusterName)
	if cluster == nil {
		return
	}
	r := reconciler.Request{Cluster: cluster.GetClientInfo(), Event: event, Obj: obj}
	r.Namespace = o.GetNamespace()
	r.Name = o.GetName()

	c.Queue.Add(r)
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

func getTargetObject(objectType runtime.Object) runtime.Object {
	switch objectType.(type) {
	case *v1.ConfigMap:
		return &v1.ConfigMap{}
	case *v1.Pod:
		return &v1.Pod{}
	case *v1.Secret:
		return &v1.Secret{}
	case *v1.Service:
		return &v1.Service{}
	case *v1.ServiceAccount:
		return &v1.ServiceAccount{}
	default:
		return nil
	}
}
