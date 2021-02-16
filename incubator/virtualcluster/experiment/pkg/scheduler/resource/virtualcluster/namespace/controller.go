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
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler"
	schedulerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/apis/config"
	internalcache "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/cache"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/engine"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/manager"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/util"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/listener"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/plugin"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func init() {
	scheduler.VirtualClusterResourceRegister.Register(&plugin.Registration{
		ID: "namespace",
		InitFn: func(ctx *plugin.InitContext) (interface{}, error) {
			v := ctx.Context.Value(constants.InternalSchedulerEngine)
			if v == nil {
				return nil, fmt.Errorf("cannot found schedulercache in context")
			}
			return NewNamespaceController(v.(engine.Engine), ctx.Config.(*schedulerconfig.SchedulerConfiguration))
		},
	})
}

type controller struct {
	SchedulerEngine        engine.Engine
	Config                 *schedulerconfig.SchedulerConfiguration
	MultiClusterController *mc.MultiClusterController
}

func NewNamespaceController(schedulerEngine engine.Engine, config *schedulerconfig.SchedulerConfiguration) (manager.ResourceWatcher, error) {
	c := &controller{
		SchedulerEngine: schedulerEngine,
		Config:          config,
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
	return listener.NewMCControllerListener(c.MultiClusterController)
}

func (c *controller) GetMCController() *mc.MultiClusterController {
	return c.MultiClusterController
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile namespace %s for virtual cluster %s", request.Name, request.ClusterName)

	// requeue if scheduler cache is not synchronized
	vcName, vcNamespace, _, err := c.MultiClusterController.GetOwnerInfo(request.ClusterName)
	if err != nil {
		return reconciler.Result{Requeue: true}, err
	}
	if _, ok := scheduler.DirtyVirtualClusters.Load(fmt.Sprintf("%s/%s", vcNamespace, vcName)); ok {
		return reconciler.Result{Requeue: true, RequeueAfter: 5 * time.Second}, fmt.Errorf("virtual cluster %s/%s is in dirty set", vcNamespace, vcName)
	}

	nsObj, err := c.MultiClusterController.Get(request.ClusterName, "", request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		// the namespace has been removed, we should update the scheduler cache
		if err := c.SchedulerEngine.UnReserveNamespace(fmt.Sprintf("%s/%s", request.ClusterName, request.Name)); err != nil {
			return reconciler.Result{Requeue: true}, fmt.Errorf("failed to unreserve namespace %s in %s: %v", request.Name, request.ClusterName, err)
		}
		return reconciler.Result{}, nil
	}

	var quota v1.ResourceList
	quotaListObj, err := c.MultiClusterController.ListByObjectType(request.ClusterName, &v1.ResourceQuota{}, client.InNamespace(request.Name))
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, fmt.Errorf("failed to get resource quota in %s/%s: %v", request.ClusterName, request.Name, err)
		}
		quota = v1.ResourceList{
			v1.ResourceCPU:    resource.MustParse("0"),
			v1.ResourceMemory: resource.MustParse("0"),
		}
	} else {
		quotaList := quotaListObj.(*v1.ResourceQuotaList)
		quota = util.GetMaxQuota(quotaList)
	}

	namespace := nsObj.(*v1.Namespace)
	placements, quotaSlice, err := util.GetSchedulingInfo(namespace)
	if err != nil {
		return reconciler.Result{Requeue: true}, fmt.Errorf("failed to get scheduling info in %s: %v", request.Namespace, err)
	}

	expect, _ := internalcache.GetNumSlices(quota, quotaSlice)
	if placements == nil {
		if expect > 0 {
			candidate := internalcache.NewNamespace(request.ClusterName, request.Name, namespace.GetLabels(), quota, quotaSlice, nil)
			_, err := c.SchedulerEngine.ScheduleNamespace(candidate)
			if err != nil {
				return reconciler.Result{Requeue: true}, fmt.Errorf("failed to schedule namespace %s in %s: %v", request.Name, request.ClusterName, err)
			}
			// TODO: Update virtualcluster namespace with the scheduling result. If fails, call UnReserveNamespace.
		}
		return reconciler.Result{}, nil
	}
	numSched := 0
	for _, v := range placements {
		numSched = numSched + v
	}

	if expect == numSched {
		// TODO: validate placement with scheduler cache
		return reconciler.Result{}, nil
	}
	candidate := internalcache.NewNamespace(request.ClusterName, request.Name, namespace.GetLabels(), quota, quotaSlice, nil)
	_, err = c.SchedulerEngine.ReScheduleNamespace(candidate)
	if err != nil {
		return reconciler.Result{Requeue: true}, fmt.Errorf("failed to reschedule namespace %s in %s: %v", request.Name, request.ClusterName, err)
	}
	// TODO: Update virtualcluster namespace with the scheduling result. If fails, call RollBackNamespace.
	return reconciler.Result{}, nil
}
