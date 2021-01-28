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

package uwcontroller

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/errors"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

type UpwardController struct {
	// objectType is the type of object to watch.  e.g. &v1.Pod{}
	objectType runtime.Object

	// objectKind is the kind of target object this controller watched.
	objectKind string

	Options
}

// Options are the arguments for creating a new UpwardController.
type Options struct {
	JitterPeriod time.Duration
	// MaxConcurrentReconciles is the number of concurrent control loops.
	MaxConcurrentReconciles int

	Reconciler reconciler.UWReconciler
	// Queue can be used to override the default queue.
	Queue workqueue.RateLimitingInterface

	name string
}

func NewUWController(objectType runtime.Object, rc reconciler.UWReconciler, opts ...OptConfig) (*UpwardController, error) {
	kinds, _, err := scheme.Scheme.ObjectKinds(objectType)
	if err != nil || len(kinds) == 0 {
		return nil, fmt.Errorf("unknown object kind %+v", objectType)
	}

	name := fmt.Sprintf("%s-upward-controller", strings.ToLower(kinds[0].Kind))
	c := &UpwardController{
		objectType: objectType,
		objectKind: kinds[0].Kind,
		Options: Options{
			name:                    name,
			JitterPeriod:            1 * time.Second,
			MaxConcurrentReconciles: 1,
			Reconciler:              rc,
			Queue:                   workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), name),
		},
	}

	for _, opt := range opts {
		opt(&c.Options)
	}

	if c.Reconciler == nil {
		return nil, fmt.Errorf("must specify UW Reconciler")
	}

	return c, nil
}

func (c *UpwardController) Start(stop <-chan struct{}) error {
	klog.Infof("start uw-controller %s", c.name)
	defer utilruntime.HandleCrash()
	defer c.Queue.ShutDown()

	for i := 0; i < c.MaxConcurrentReconciles; i++ {
		go wait.Until(c.worker, c.JitterPeriod, stop)
	}

	<-stop
	klog.Infof("shutting down uw-controller %s", c.name)
	return nil
}

func (c *UpwardController) AddToQueue(key string) {
	c.Queue.Add(key)
}

func (c *UpwardController) worker() {
	for c.processNextWorkItem() {
	}
}

func (c *UpwardController) processNextWorkItem() bool {
	obj, quit := c.Queue.Get()
	if quit {
		return false
	}
	defer c.Queue.Done(obj)

	key, ok := obj.(string)
	if !ok {
		c.Queue.Forget(obj)
		return true
	}

	defer metrics.RecordUWSOperationDuration(c.objectKind, time.Now())

	klog.V(4).Infof("%s back populate %+v", c.name, key)
	err := c.Reconciler.BackPopulate(key)
	if err == nil {
		metrics.RecordUWSOperationStatus(c.objectKind, constants.StatusCodeOK)
		c.Queue.Forget(obj)
		return true
	}

	if errors.IsClusterNotFound(err) {
		// The virtual cluster has been removed, do not reconcile for its uws requests.
		klog.Warningf("%v, drop the uws request %v", err.Error(), key)
		c.Queue.Forget(obj)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%s error processing %s (will retry): %v", c.name, key, err))
	if c.Queue.NumRequeues(key) >= constants.MaxReconcileRetryAttempts {
		metrics.RecordUWSOperationStatus(c.objectKind, constants.StatusCodeExceedMaxRetryAttempts)
		klog.Warningf("%s uws request is dropped due to reaching max retry limit: %s", c.name, key)
		c.Queue.Forget(obj)
		return true
	}
	metrics.RecordUWSOperationStatus(c.objectKind, constants.StatusCodeError)
	c.Queue.AddRateLimited(obj)
	return true
}
