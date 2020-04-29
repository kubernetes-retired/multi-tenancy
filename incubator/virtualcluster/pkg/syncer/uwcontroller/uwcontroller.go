/*
Copyright 2020 The Kubernetes Authors.

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
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type UpwardController struct {
	name string

	// objectType is the type of object to watch.  e.g. &v1.Pod{}
	objectType runtime.Object

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
}

func NewUWController(name string, objectType runtime.Object, options Options) (*UpwardController, error) {
	if options.Reconciler == nil {
		return nil, fmt.Errorf("must specify UW Reconciler")
	}

	if len(name) == 0 {
		return nil, fmt.Errorf("must specify Name for Controller")
	}

	c := &UpwardController{
		name:       name,
		objectType: objectType,
		Options:    options,
	}

	if c.JitterPeriod == 0 {
		c.JitterPeriod = 1 * time.Second
	}

	if c.MaxConcurrentReconciles <= 0 {
		c.MaxConcurrentReconciles = 1
	}

	if c.Queue == nil {
		c.Queue = workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), c.name)
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

	klog.V(4).Infof("%s back populate %+v", c.name, key)
	err := c.Reconciler.BackPopulate(key)
	if err == nil {
		c.Queue.Forget(obj)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("%s error processing %s (will retry): %v", c.name, key, err))
	if c.Queue.NumRequeues(key) >= constants.MaxUwsRetryAttempts {
		klog.Warningf("%s uws request is dropped due to reaching max retry limit: %s", c.name, key)
		c.Queue.Forget(obj)
		return true
	}
	c.Queue.AddRateLimited(obj)
	return true
}
