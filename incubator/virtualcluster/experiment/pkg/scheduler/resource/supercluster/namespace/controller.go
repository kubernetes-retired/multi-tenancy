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

package namespace

import (
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler"
	schedulerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/apis/config"
	internalcache "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/cache"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/manager"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/util"
	syncerconst "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/plugin"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func init() {
	scheduler.SuperClusterResourceRegister.Register(&plugin.Registration{
		ID: "namespace",
		InitFn: func(ctx *plugin.InitContext) (interface{}, error) {
			v := ctx.Context.Value(constants.InternalSchedulerCache)
			if v == nil {
				return nil, fmt.Errorf("cannot found schedulercache in context")
			}
			return NewNamespaceController(v.(internalcache.Cache), ctx.Config.(*schedulerconfig.SchedulerConfiguration))
		},
	})
}

type controller struct {
	SchedulerCache         internalcache.Cache
	Config                 *schedulerconfig.SchedulerConfiguration
	MultiClusterController *mc.MultiClusterController
}

func NewNamespaceController(schedulerCache internalcache.Cache, config *schedulerconfig.SchedulerConfiguration) (manager.ResourceWatcher, error) {
	c := &controller{
		SchedulerCache: schedulerCache,
		Config:         config,
	}

	var err error
	c.MultiClusterController, err = mc.NewMCController(&v1.Namespace{}, &v1.NamespaceList{}, c)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *controller) Start(stopCh <-chan struct{}) error {
	return c.MultiClusterController.Start(stopCh)
}

func (c *controller) GetListener() listener.ClusterChangeListener {
	return listener.NewMCControllerListener(c.MultiClusterController, mc.WatchOptions{})
}

func (c *controller) GetMCController() *mc.MultiClusterController {
	return c.MultiClusterController
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile namespace %s for super cluster %s", request.Name, request.ClusterName)
	key := fmt.Sprintf("%s/%s", request.ClusterName, request.Name)
	exists := true
	nsObj, err := c.MultiClusterController.Get(request.ClusterName, request.Namespace, request.Name)
	ns := nsObj.(*v1.Namespace)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		exists = false
	}

	if exists {
		if _, ok := ns.GetAnnotations()[syncerconst.LabelCluster]; !ok {
			// this is not a namespace created by the syncer
			return reconciler.Result{}, nil
		}
		var slices []*internalcache.Slice
		slices, err = util.GetProvisionedSlices(ns, request.ClusterName, key)
		if err != nil {
			return reconciler.Result{Requeue: true}, fmt.Errorf("fail to reconcile %s/%s: %v", request.ClusterName, request.Name, err)
		}
		if err = c.SchedulerCache.AddProvision(request.ClusterName, key, slices); err != nil {
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		if err = c.SchedulerCache.RemoveProvision(request.ClusterName, key); err != nil {
			return reconciler.Result{Requeue: true}, err
		}
	}
	return reconciler.Result{}, nil
}
