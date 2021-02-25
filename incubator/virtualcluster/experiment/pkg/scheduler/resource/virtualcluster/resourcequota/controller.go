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

package resourcequota

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler"
	schedulerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/manager"

	//syncerconst "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/plugin"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func init() {
	scheduler.VirtualClusterResourceRegister.Register(&plugin.Registration{
		ID: "resourcequota",
		InitFn: func(ctx *plugin.InitContext) (interface{}, error) {
			v := ctx.Context.Value(constants.InternalSchedulerManager)
			if v == nil {
				return nil, fmt.Errorf("cannot found schedulercache in context")
			}
			return NewResourceQuotaController(v.(*manager.WatchManager), ctx.Config.(*schedulerconfig.SchedulerConfiguration))
		},
	})
}

type controller struct {
	SchedulerManager       *manager.WatchManager
	Config                 *schedulerconfig.SchedulerConfiguration
	MultiClusterController *mc.MultiClusterController
}

func NewResourceQuotaController(mgr *manager.WatchManager, config *schedulerconfig.SchedulerConfiguration) (manager.ResourceWatcher, error) {
	c := &controller{
		SchedulerManager: mgr,
		Config:           config,
	}

	var err error
	c.MultiClusterController, err = mc.NewMCController(&v1.ResourceQuota{}, &v1.ResourceQuotaList{}, c)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (c *controller) Start(stopCh <-chan struct{}) error {
	return c.MultiClusterController.Start(stopCh)
}

func (c *controller) GetMCController() *mc.MultiClusterController {
	return c.MultiClusterController
}

func (c *controller) GetListener() listener.ClusterChangeListener {
	return listener.NewMCControllerListener(c.MultiClusterController, mc.WatchOptions{})
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile resource quota %s/%s for virtual cluster %s", request.Namespace, request.Namespace, request.ClusterName)

	// forward the request to namespace reconciler
	nsWatcher := c.SchedulerManager.GetResourceWatcherByMCControllerName("namespace-mccontroller")
	if nsWatcher == nil {
		panic("namespace mccontroller is necessary")
	}

	nsObj, err := c.MultiClusterController.GetByObjectType(request.ClusterName, "", request.Namespace, &v1.Namespace{})
	if err != nil {
		return reconciler.Result{Requeue: true}, fmt.Errorf("failed to get namespace %s in %s: %v", request.Namespace, request.ClusterName, err)
	}
	namespace := nsObj.(*v1.Namespace)
	if err := nsWatcher.GetMCController().RequeueObject(request.ClusterName, namespace); err != nil {
		return reconciler.Result{Requeue: true}, fmt.Errorf("failed to requeue namespace %s in %s: %v", request.Namespace, request.ClusterName, err)
	}
	return reconciler.Result{}, nil
}
