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
	"context"
	"encoding/json"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler"
	schedulerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/apis/config"
	internalcache "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/cache"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/engine"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/manager"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/util"
	utilconst "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
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
	return listener.NewMCControllerListener(c.MultiClusterController, mc.WatchOptions{})
}

func (c *controller) GetMCController() *mc.MultiClusterController {
	return c.MultiClusterController
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile namespace %s for virtual cluster %s", request.Name, request.ClusterName)

	// requeue if scheduler cache is not synchronized
	vcName, vcNamespace, _, err := c.MultiClusterController.GetOwnerInfo(request.ClusterName)
	if err != nil {
		return reconciler.Result{}, err
	}
	if _, ok := scheduler.DirtyVirtualClusters.Load(fmt.Sprintf("%s/%s", vcNamespace, vcName)); ok {
		klog.Warningf("virtual cluster %s/%s is in dirty set", vcNamespace, vcName)
		return reconciler.Result{RequeueAfter: 5 * time.Second}, nil
	}

	nsObj, err := c.MultiClusterController.Get(request.ClusterName, "", request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{}, err
		}
		klog.Infof("namespace %s/%s is removed", request.ClusterName, request.Name)
		// the namespace has been removed, we should update the scheduler cache
		if err := c.SchedulerEngine.DeScheduleNamespace(fmt.Sprintf("%s/%s", request.ClusterName, request.Name)); err != nil {
			return reconciler.Result{}, fmt.Errorf("failed to unreserve namespace %s in %s: %v", request.Name, request.ClusterName, err)
		}
		return reconciler.Result{}, nil
	}

	var quota v1.ResourceList
	quotaListObj, err := c.MultiClusterController.ListByObjectType(request.ClusterName, &v1.ResourceQuota{}, client.InNamespace(request.Name))
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{}, fmt.Errorf("failed to get resource quota in %s/%s: %v", request.ClusterName, request.Name, err)
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
		return reconciler.Result{}, fmt.Errorf("failed to get scheduling info in %s: %v", request.Name, err)
	}

	expect, _ := internalcache.GetLeastFitSliceNum(quota, quotaSlice)
	if expect == 0 {
		// the quota is gone. we should update the scheduler cache
		if err := c.SchedulerEngine.DeScheduleNamespace(fmt.Sprintf("%s/%s", request.ClusterName, request.Name)); err != nil {
			return reconciler.Result{}, fmt.Errorf("failed to unreserve namespace %s in %s: %v", request.Name, request.ClusterName, err)
		}
		return reconciler.Result{}, nil

	}
	numSched := 0
	var schedule []*internalcache.Placement
	for k, v := range placements {
		numSched = numSched + v
		schedule = append(schedule, internalcache.NewPlacement(k, v))
	}

	candidate := internalcache.NewNamespace(request.ClusterName, request.Name, namespace.GetLabels(), quota, quotaSlice, schedule)
	// ensure the cache is consistent with the scheduled placements
	if numSched == expect {
		if err := c.SchedulerEngine.EnsureNamespacePlacements(candidate); err != nil {
			return reconciler.Result{}, fmt.Errorf("failed to ensure namespace %s's placements in %s: %v", request.Name, request.ClusterName, err)
		}
		return reconciler.Result{}, nil
	}

	// some (or all) slices need to be scheduled/rescheduled
	ret, err := c.SchedulerEngine.ScheduleNamespace(candidate)
	if err != nil {
		c.MultiClusterController.Eventf(request.ClusterName, &v1.ObjectReference{
			Kind:      "Namespace",
			Name:      namespace.Name,
			Namespace: namespace.Name,
			UID:       namespace.UID,
		}, v1.EventTypeNormal, "Failed", "Failed to schedule namespace %s: %v", request.Name, err)
		return reconciler.Result{}, fmt.Errorf("failed to schedule namespace %s in %s: %v", request.Name, request.ClusterName, err)
	}
	// update virtualcluster namespace with the scheduling result.
	vcClient, err := c.MultiClusterController.GetClusterClient(request.ClusterName)
	if err != nil {
		return reconciler.Result{}, fmt.Errorf("failed to get vc %s's client: %v", request.ClusterName, err)
	}
	updatedPlacement, _ := json.Marshal(ret.GetPlacementMap())
	clone := namespace.DeepCopy()
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if clone.Annotations == nil {
			clone.Annotations = make(map[string]string)
		}
		clone.Annotations[utilconst.LabelScheduledPlacements] = string(updatedPlacement)
		_, updateErr := vcClient.CoreV1().Namespaces().Update(context.TODO(), clone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		if got, err := vcClient.CoreV1().Namespaces().Get(context.TODO(), clone.Name, metav1.GetOptions{}); err == nil {
			clone = got
		}
		return updateErr
	})
	if err == nil {
		klog.Infof("Successfully schedule namespace %s/%s with placement %s", request.ClusterName, request.Name, string(updatedPlacement))
		err = c.MultiClusterController.Eventf(request.ClusterName, &v1.ObjectReference{
			Kind:      "Namespace",
			Name:      namespace.Name,
			Namespace: namespace.Name,
			UID:       namespace.UID,
		}, v1.EventTypeNormal, "Scheduled", "Successfully schedule namespace %s with placement %s", request.Name, string(updatedPlacement))
	}
	return reconciler.Result{}, err
}
