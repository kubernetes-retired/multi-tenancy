/*
Copyright 2021 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
        "k8s.io/client-go/rest"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientgocache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/featuregate"
	utilscheme "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/scheme"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/errors"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/fairqueue"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/handler"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

// MultiClusterController implements the multicluster controller pattern.
// A MultiClusterController owns a client-go workqueue. The WatchClusterResource methods set
// up the queue to receive reconcile requests, e.g., CRUD events from a tenant cluster.
// The Requests are processed by the user-provided Reconciler.
// MultiClusterController saves all watched tenant clusters in a set.
type MultiClusterController struct {
	sync.Mutex

	// objectType is the type of object to watch.  e.g. &v1.Pod{}
	objectType runtime.Object

	// objectKind is the kind of target object this controller watched.
	objectKind string

	// clusters is the internal cluster set this controller watches.
	clusters map[string]ClusterInterface

	Options
}

// Options are the arguments for creating a new Controller.
type Options struct {
	// JitterPeriod is the time to wait after an error to start working again.
	JitterPeriod time.Duration

	// MaxConcurrentReconciles is the number of concurrent control loops.
	MaxConcurrentReconciles int

	Reconciler reconciler.DWReconciler

	// Queue can be used to override the default queue.
	Queue workqueue.RateLimitingInterface

	// name is used to uniquely identify a Controller in tracing, logging and monitoring.  Name is required.
	name string
}

// Cache is the interface used by Controller to start and wait for caches to sync.
type Cache interface {
	Start() error
	WaitForCacheSync() bool
	Stop()
}

// Lister interface is used to get the CRD object that abstracts the cluster.
type Getter interface {
	GetObject(string, string) (runtime.Object, error)
}

// ClusterInterface decouples the controller package from the cluster package.
type ClusterInterface interface {
	GetClusterName() string
	GetOwnerInfo() (string, string, string)
	GetObject() (runtime.Object, error)
	AddEventHandler(runtime.Object, clientgocache.ResourceEventHandler) error
	GetInformer(objectType runtime.Object) (cache.Informer, error)
	GetClientSet() (clientset.Interface, error)
	GetDelegatingClient() (client.Client, error)
        GetRestConfig() *rest.Config
	Cache
}

// NewMCController creates a new MultiClusterController.
func NewMCController(objectType, objectListType runtime.Object, rc reconciler.DWReconciler, opts ...OptConfig) (*MultiClusterController, error) {
	kinds, _, err := scheme.Scheme.ObjectKinds(objectType)
	if err != nil || len(kinds) == 0 {
		return nil, fmt.Errorf("unknown object kind %+v", objectType)
	}

	c := &MultiClusterController{
		objectType: objectType,
		objectKind: kinds[0].Kind,
		clusters:   make(map[string]ClusterInterface),
		Options: Options{
			name:                    fmt.Sprintf("%s-mccontroller", strings.ToLower(kinds[0].Kind)),
			JitterPeriod:            1 * time.Second,
			MaxConcurrentReconciles: 1,
			Reconciler:              rc,
			Queue:                   fairqueue.NewRateLimitingFairQueue(),
		},
	}

	utilscheme.Scheme.AddKnownTypePair(objectType, objectListType)

	for _, opt := range opts {
		opt(&c.Options)
	}

	if c.Reconciler == nil {
		return nil, fmt.Errorf("must specify DW Reconciler")
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
	if _, exist := c.clusters[cluster.GetClusterName()]; !exist {
		return fmt.Errorf("please register cluster %s resource before watch", cluster.GetClusterName())
	}

	if c.objectType == nil {
		return nil
	}

	h := &handler.EnqueueRequestForObject{ClusterName: cluster.GetClusterName(), Queue: c.Queue}
	return cluster.AddEventHandler(c.objectType, h)
}

// RegisterClusterResource get the informer *before* trying to wait for the
// caches to sync so that we have a chance to register their intended caches.
func (c *MultiClusterController) RegisterClusterResource(cluster ClusterInterface, o WatchOptions) error {
	c.Lock()
	defer c.Unlock()
	if _, exist := c.clusters[cluster.GetClusterName()]; exist {
		return nil
	}
	c.clusters[cluster.GetClusterName()] = cluster

	if c.objectType == nil {
		return nil
	}

	_, err := cluster.GetInformer(c.objectType)
	return err
}

// TeardownClusterResource forget the cluster it watches.
// The cluster informer should stop together.
func (c *MultiClusterController) TeardownClusterResource(cluster ClusterInterface) {
	c.Lock()
	defer c.Unlock()
	delete(c.clusters, cluster.GetClusterName())
}

// Start starts the ClustersController's control loops (as many as MaxConcurrentReconciles) in separate channels
// and blocks until an empty struct is sent to the stop channel.
func (c *MultiClusterController) Start(stop <-chan struct{}) error {
	klog.Infof("start mc-controller %q", c.name)

	defer c.Queue.ShutDown()

	for i := 0; i < c.MaxConcurrentReconciles; i++ {
		go wait.Until(c.worker, c.JitterPeriod, stop)
	}

	select {
	case <-stop:
		return nil
	}
}

// GetControllerName get the mccontroller name, is used to uniquely identify the Controller in tracing, logging and monitoring.
func (c *MultiClusterController) GetControllerName() string {
	return c.name
}

// GetObjectKind is the objectKind name this controller watch to.
func (c *MultiClusterController) GetObjectKind() string {
	return c.objectKind
}

// Get returns object with specific cluster, namespace and name.
func (c *MultiClusterController) Get(clusterName, namespace, name string) (interface{}, error) {
	cluster := c.getCluster(clusterName)
	if cluster == nil {
		return nil, errors.NewClusterNotFound(clusterName)
	}
	instance := utilscheme.Scheme.NewObject(c.objectType)
	delegatingClient, err := cluster.GetDelegatingClient()
	if err != nil {
		return nil, err
	}
	err = delegatingClient.Get(context.TODO(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, instance)
	return instance, err
}

// GetByObjectType returns object with specific cluster, namespace and name and object type
func (c *MultiClusterController) GetByObjectType(clusterName, namespace, name string, objectType runtime.Object) (interface{}, error) {
	cluster := c.getCluster(clusterName)
	if cluster == nil {
		return nil, errors.NewClusterNotFound(clusterName)
	}
	instance := utilscheme.Scheme.NewObject(objectType)
	if instance == nil {
		return nil, fmt.Errorf("the object type %v is not supported by mccontroller", objectType)
	}
	delegatingClient, err := cluster.GetDelegatingClient()
	if err != nil {
		return nil, err
	}
	err = delegatingClient.Get(context.TODO(), client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, instance)
	return instance, err
}

// List returns a list of objects with specific cluster.
func (c *MultiClusterController) List(clusterName string) (interface{}, error) {
	cluster := c.getCluster(clusterName)
	if cluster == nil {
		return nil, errors.NewClusterNotFound(clusterName)
	}
	instanceList := utilscheme.Scheme.NewObjectList(c.objectType)
	delegatingClient, err := cluster.GetDelegatingClient()
	if err != nil {
		return nil, err
	}
	err = delegatingClient.List(context.TODO(), instanceList)
	return instanceList, err
}

// ListByObjectType returns a list of objects with specific cluster and object type.
func (c *MultiClusterController) ListByObjectType(clusterName string, objectType runtime.Object) (interface{}, error) {
	cluster := c.getCluster(clusterName)
	if cluster == nil {
		return nil, errors.NewClusterNotFound(clusterName)
	}
	instanceList := utilscheme.Scheme.NewObjectList(objectType)
	if instanceList == nil {
		return nil, fmt.Errorf("the object type %v is not supported by mccontroller", objectType)
	}
	delegatingClient, err := cluster.GetDelegatingClient()
	if err != nil {
		return nil, err
	}
	err = delegatingClient.List(context.TODO(), instanceList)
	return instanceList, err
}

func (c *MultiClusterController) getCluster(clusterName string) ClusterInterface {
	c.Lock()
	defer c.Unlock()
	return c.clusters[clusterName]
}

// GetClusterClient returns the cluster's clientset client for direct access to tenant apiserver
func (c *MultiClusterController) GetClusterClient(clusterName string) (clientset.Interface, error) {
	cluster := c.getCluster(clusterName)
	if cluster == nil {
		return nil, errors.NewClusterNotFound(clusterName)
	}
	return cluster.GetClientSet()
}

func (c *MultiClusterController) GetClusterObject(clusterName string) (runtime.Object, error) {
	cluster := c.getCluster(clusterName)
	if cluster == nil {
		return nil, errors.NewClusterNotFound(clusterName)
	}
	obj, err := cluster.GetObject()
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func (c *MultiClusterController) GetOwnerInfo(clusterName string) (string, string, string, error) {
	cluster := c.getCluster(clusterName)
	if cluster == nil {
		return "", "", "", errors.NewClusterNotFound(clusterName)
	}
	name, namespace, uid := cluster.GetOwnerInfo()
	return name, namespace, uid, nil
}

// GetClusterNames returns the name list of all managed tenant clusters
func (c *MultiClusterController) GetClusterNames() []string {
	c.Lock()
	defer c.Unlock()
	var names []string
	for _, cluster := range c.clusters {
		names = append(names, cluster.GetClusterName())
	}
	return names
}

// Eventf constructs an event from the given information and puts it in the queue for sending.
// 'ref' is the object this event is about. Event will make a reference or you may also
// pass a reference to the object directly.
// 'eventtype' of this event, and can be one of Normal, Warning. New types could be added in future
// 'reason' is the reason this event is generated. 'reason' should be short and unique; it
// should be in UpperCamelCase format (starting with a capital letter). "reason" will be used
// to automate handling of events, so imagine people writing switch statements to handle them.
// You want to make that easy.
// 'message' is intended to be human readable.
//
// The resulting event will be created in the same namespace as the reference object.
// TODO(zhuangqh): consider maintain an event sink for each tenant.
func (c *MultiClusterController) Eventf(clusterName string, ref *v1.ObjectReference, eventtype string, reason, messageFmt string, args ...interface{}) error {
	tenantClient, err := c.GetClusterClient(clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}
	namespace := ref.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	eventTime := metav1.Now()
	event := &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%v.%x", ref.Name, eventTime.UnixNano()),
			Namespace: namespace,
		},
		InvolvedObject:      *ref,
		Type:                eventtype,
		Reason:              reason,
		Message:             fmt.Sprintf(messageFmt, args...),
		FirstTimestamp:      eventTime,
		LastTimestamp:       eventTime,
		ReportingController: "vc-syncer",
	}
	_, err = tenantClient.CoreV1().Events(namespace).Create(context.TODO(), event, metav1.CreateOptions{})
	return err
}

// RequeueObject requeues the cluster object, thus reconcileHandler can reconcile it again.
func (c *MultiClusterController) RequeueObject(clusterName string, obj interface{}) error {
	o, err := meta.Accessor(obj)
	if err != nil {
		return err
	}

	cluster := c.getCluster(clusterName)
	if cluster == nil {
		return errors.NewClusterNotFound(clusterName)
	}
	//FIXME: we dont need event here.
	r := reconciler.Request{}
	r.ClusterName = clusterName
	r.Namespace = o.GetNamespace()
	r.Name = o.GetName()
	r.UID = string(o.GetUID())

	c.Queue.Add(r)
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
		klog.V(4).Info("Shutting down. Ignore work item and stop working.")
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
	if c.getCluster(req.ClusterName) == nil {
		// The virtual cluster has been removed, do not reconcile for its dws requests.
		klog.Warningf("The cluster %s has been removed, drop the dws request %v", req.ClusterName, req)
		c.Queue.Forget(obj)
		return true
	}

	if featuregate.DefaultFeatureGate.Enabled(featuregate.SuperClusterPooling) {
		if c.FilterObjectFromSchedulingResult(req) {
			c.Queue.Forget(req)
			c.Queue.Done(req)
			klog.Infof("drop request %+v which doesn't scheduled to this cluster", req)
			return true
		}
	}

	defer metrics.RecordDWSOperationDuration(c.objectKind, req.ClusterName, time.Now())

	// RunInformersAndControllers the syncHandler, passing it the cluster/namespace/Name
	// string of the resource to be synced.
	result, err := c.Reconciler.Reconcile(req)
	if err == nil {
		metrics.RecordDWSOperationStatus(c.objectKind, req.ClusterName, constants.StatusCodeOK)
		if result.RequeueAfter > 0 {
			c.Queue.AddAfter(req, result.RequeueAfter)
		} else if result.Requeue {
			c.Queue.AddRateLimited(req)
		}
		// if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.Queue.Forget(obj)
		return true
	}

	// rejected by apiserver(maybe rejected by webhook or other admission plugins)
	// we take a negative attitude on this situation and fail fast.
	if apierr, ok := err.(apierrors.APIStatus); ok {
		if code := apierr.Status().Code; code == http.StatusBadRequest || code == http.StatusForbidden {
			metrics.RecordDWSOperationStatus(c.objectKind, req.ClusterName, constants.StatusCodeBadRequest)
			klog.Errorf("%s dws request is rejected: %v", c.name, err)
			c.Queue.Forget(obj)
			return true
		}
	}

	// exceed max retry
	if c.Queue.NumRequeues(obj) >= constants.MaxReconcileRetryAttempts {
		metrics.RecordDWSOperationStatus(c.objectKind, req.ClusterName, constants.StatusCodeExceedMaxRetryAttempts)
		c.Queue.Forget(obj)
		klog.Warningf("%s dws request is dropped due to reaching max retry limit: %+v", c.name, obj)
		return true
	}

	metrics.RecordDWSOperationStatus(c.objectKind, req.ClusterName, constants.StatusCodeError)
	c.Queue.AddRateLimited(req)
	klog.Errorf("%s dws request reconcile failed: %v", req, err)
	return false
}

func (c *MultiClusterController) FilterObjectFromSchedulingResult(req reconciler.Request) bool {
	var nsName string
	if c.objectKind == "Namespace" {
		nsName = req.Name
	} else {
		nsName = req.Namespace
	}

	if filterSuperClusterRelatedObject(c, req.ClusterName, nsName) {
		return true
	}

	if c.objectKind == "Pod" {
		if filterSuperClusterSchedulePod(c, req) {
			return true
		}
	}

	return false
}

func filterSuperClusterRelatedObject(c *MultiClusterController, clusterName, nsName string) bool {
	nsObj, err := c.GetByObjectType(clusterName, "", nsName, &v1.Namespace{})
	if err != nil {
		klog.Errorf("failed to get ns %s of cluster %s: %v", nsName, clusterName, err)
		return true
	}
	placements := make(map[string]int)
	clist, ok := nsObj.(*v1.Namespace).GetAnnotations()[constants.LabelScheduledPlacements]
	if !ok {
		return true
	}
	if err = json.Unmarshal([]byte(clist), &placements); err != nil {
		klog.Errorf("unknown format %s of key %s, cluster %s, ns %s: %v", clist, constants.LabelScheduledPlacements, clusterName, nsName, err)
		return true
	}

	_, ok = placements[constants.SuperClusterID]

	return !ok
}

func filterSuperClusterSchedulePod(c *MultiClusterController, req reconciler.Request) bool {
	podObj, err := c.GetByObjectType(req.ClusterName, req.Namespace, req.Name, &v1.Pod{})
	if err != nil {
		klog.Errorf("failed to get pod %+v: %v", req, err)
		return true
	}

	cname, ok := podObj.(*v1.Pod).GetAnnotations()[constants.LabelScheduledCluster]
	if !ok {
		return true
	}

	return cname != constants.SuperClusterID
}
