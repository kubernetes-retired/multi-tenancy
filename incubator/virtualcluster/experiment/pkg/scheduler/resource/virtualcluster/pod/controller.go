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

package pod

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"

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
		ID: "pod",
		InitFn: func(ctx *plugin.InitContext) (interface{}, error) {
			v := ctx.Context.Value(constants.InternalSchedulerEngine)
			if v == nil {
				return nil, fmt.Errorf("cannot found schedulercache in context")
			}
			return NewPodController(v.(engine.Engine), ctx.Config.(*schedulerconfig.SchedulerConfiguration))
		},
	})
}

type controller struct {
	SchedulerEngine        engine.Engine
	Config                 *schedulerconfig.SchedulerConfiguration
	MultiClusterController *mc.MultiClusterController
}

func NewPodController(schedulerEngine engine.Engine, config *schedulerconfig.SchedulerConfiguration) (manager.ResourceWatcher, error) {
	c := &controller{
		SchedulerEngine: schedulerEngine,
		Config:          config,
	}

	var err error
	c.MultiClusterController, err = mc.NewMCController(&v1.Pod{}, &v1.PodList{}, c)
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
	klog.Infof("reconcile pod %s for virtual cluster %s", request.Name, request.ClusterName)

	// requeue if scheduler cache is not synchronized
	vcName, vcNamespace, _, err := c.MultiClusterController.GetOwnerInfo(request.ClusterName)
	if err != nil {
		return reconciler.Result{}, err
	}
	if _, ok := scheduler.DirtyVirtualClusters.Load(fmt.Sprintf("%s/%s", vcNamespace, vcName)); ok {
		klog.Warningf("virtual cluster %s/%s is in dirty set", vcNamespace, vcName)
		return reconciler.Result{RequeueAfter: 5 * time.Second}, nil
	}

	podKey := fmt.Sprintf("%s/%s/%s", request.ClusterName, request.Namespace, request.Name)
	podObj, err := c.MultiClusterController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{}, err
		}

		if err := c.SchedulerEngine.DeSchedulePod(podKey); err != nil {
			return reconciler.Result{}, fmt.Errorf("failed to unreserve pod %s in %s: %v", request.Name, request.ClusterName, err)
		}
		return reconciler.Result{}, nil
	}

	pod := podObj.(*v1.Pod)
	if c.skipPodSchedule(pod) {
		// skip irrelevant pod update event
		// we assume pod's scheduling info won't be manually mutated during pod running by now.
		return reconciler.Result{}, nil
	}

	candidate := internalcache.NewPod(request.ClusterName, pod.Namespace, pod.Name, "", util.GetPodRequirements(pod))
	ret, err := c.SchedulerEngine.SchedulePod(candidate)
	if err != nil {
		c.MultiClusterController.Eventf(request.ClusterName, &v1.ObjectReference{
			Kind:      "Pod",
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       pod.UID,
		}, v1.EventTypeNormal, "Failed", "failed to schedule pod %s/%s to any cluster: %v", request.Namespace, request.Name, err)
		return reconciler.Result{}, fmt.Errorf("failed to schedule pod %s in %s: %v", request.Name, request.ClusterName, err)
	}

	// update virtualcluster pod with the scheduling result.
	vcClient, err := c.MultiClusterController.GetClusterClient(request.ClusterName)
	if err != nil {
		return reconciler.Result{}, fmt.Errorf("failed to get vc %s's client: %v", request.ClusterName, err)
	}

	clone := pod.DeepCopy()
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if clone.Annotations == nil {
			clone.Annotations = make(map[string]string)
		}
		clone.Annotations[utilconst.LabelScheduledCluster] = ret.GetCluster()
		_, updateErr := vcClient.CoreV1().Pods(clone.Namespace).Update(context.TODO(), clone, metav1.UpdateOptions{})
		if updateErr == nil {
			return nil
		}
		if got, err := vcClient.CoreV1().Pods(clone.Namespace).Get(context.TODO(), clone.Name, metav1.GetOptions{}); err == nil {
			clone = got
		}
		return updateErr
	})
	if err == nil {
		klog.Infof("Successfully schedule pod %s with placement %s", ret.GetKey(), ret.GetCluster())
		err = c.MultiClusterController.Eventf(request.ClusterName, &v1.ObjectReference{
			Kind:      "Pod",
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       pod.UID,
		}, v1.EventTypeNormal, "Scheduled", "Successfully schedule pod %s to cluster %s", ret.GetKey(), ret.GetCluster())
	}
	return reconciler.Result{}, err
}

func (c *controller) skipPodSchedule(pod *v1.Pod) bool {
	if pod.DeletionTimestamp != nil {
		klog.Infof("skip schedule deleting pod %s/%s", pod.GetNamespace(), pod.GetName())
		return true
	}

	return util.GetPodSchedulingInfo(pod) != ""
}
