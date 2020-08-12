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

package priorityclass

import (
	"context"
	"fmt"
	"k8s.io/klog"

	v1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.priorityclassSynced) {
		return fmt.Errorf("failed to wait for caches to sync priorityclass")
	}
	return c.upwardPriorityClassController.Start(stopCh)
}

func (c *controller) BackPopulate(key string) error {
	// The key format is clsutername/scName.
	clusterName, scName, _ := cache.SplitMetaNamespaceKey(key)
	klog.V(4).Infof("backPopulate key %v for clusterName %v", key, clusterName)
	op := reconciler.AddEvent
	pPriorityClass, err := c.priorityclassLister.Get(scName)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
		op = reconciler.DeleteEvent
	}

	tenantClient, err := c.multiClusterPriorityClassController.GetClusterClient(clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}

	vPriorityClassObj, err := c.multiClusterPriorityClassController.Get(clusterName, "", scName)
	if err != nil {
		if errors.IsNotFound(err) {
			if op == reconciler.AddEvent {
				// Available in super, hence create a new in tenant master
				vPriorityClass := conversion.BuildVirtualPriorityClass(clusterName, pPriorityClass)
				_, err := tenantClient.SchedulingV1().PriorityClasses().Create(context.TODO(), vPriorityClass, metav1.CreateOptions{})
				if err != nil {
					return err
				}
			}
			return nil
		}
		return err
	}

	if op == reconciler.DeleteEvent {
		opts := &metav1.DeleteOptions{
			PropagationPolicy: &constants.DefaultDeletionPolicy,
		}
		err := tenantClient.SchedulingV1().PriorityClasses().Delete(context.TODO(), scName, *opts)
		if err != nil {
			return err
		}
	} else {
		updatedPriorityClass := conversion.Equality(c.config, nil).CheckPriorityClassEquality(pPriorityClass, vPriorityClassObj.(*v1.PriorityClass))
		if updatedPriorityClass != nil {
			_, err := tenantClient.SchedulingV1().PriorityClasses().Update(context.TODO(), updatedPriorityClass, metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
