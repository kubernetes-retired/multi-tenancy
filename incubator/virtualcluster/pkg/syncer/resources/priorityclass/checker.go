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

package priorityclass

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	v1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
)

var numMissMatchedPriorityClasses uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.priorityclassSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service checker")
	}
	c.Patroller.Start(stopCh)
	return nil
}

// ParollerDo check if PriorityClass keeps consistency between super master and tenant masters.
func (c *controller) PatrollerDo() {
	clusterNames := c.MultiClusterController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.V(2).Infof("tenant masters has no clusters, give up priority class period checker")
		return
	}

	wg := sync.WaitGroup{}
	numMissMatchedPriorityClasses = 0

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkPriorityClassOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pPriorityClassList, err := c.priorityclassLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing priorityclass from super master informer cache: %v", err)
		return
	}

	for _, pPriorityClass := range pPriorityClassList {
		if !publicPriorityClass(pPriorityClass) {
			continue
		}
		for _, clusterName := range clusterNames {
			_, err := c.MultiClusterController.Get(clusterName, "", pPriorityClass.Name)
			if err != nil {
				if errors.IsNotFound(err) {
					metrics.CheckerRemedyStats.WithLabelValues("RequeuedSuperMasterPriorityClasses").Inc()
					c.UpwardController.AddToQueue(clusterName + "/" + pPriorityClass.Name)
				}
				klog.Errorf("fail to get priorityclass from cluster %s: %v", clusterName, err)
			}
		}
	}

	metrics.CheckerMissMatchStats.WithLabelValues("MissMatchedPriorityClasses").Set(float64(numMissMatchedPriorityClasses))
}

func (c *controller) checkPriorityClassOfTenantCluster(clusterName string) {
	listObj, err := c.MultiClusterController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing priorityclass from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.V(4).Infof("check priorityclass consistency in cluster %s", clusterName)
	scList := listObj.(*v1.PriorityClassList)
	for i, vPriorityClass := range scList.Items {
		pPriorityClass, err := c.priorityclassLister.Get(vPriorityClass.Name)
		if errors.IsNotFound(err) {
			// super master is the source of the truth for sc object, delete tenant master obj
			tenantClient, err := c.MultiClusterController.GetClusterClient(clusterName)
			if err != nil {
				klog.Errorf("error getting cluster %s clientset: %v", clusterName, err)
				continue
			}
			opts := &metav1.DeleteOptions{
				PropagationPolicy: &constants.DefaultDeletionPolicy,
			}
			if err := tenantClient.SchedulingV1().PriorityClasses().Delete(context.TODO(), vPriorityClass.Name, *opts); err != nil {
				klog.Errorf("error deleting priorityclass %v in cluster %s: %v", vPriorityClass.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanTenantPriorityClasses").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("failed to get pPriorityClass %s from super master cache: %v", vPriorityClass.Name, err)
			continue
		}

		updatedPriorityClass := conversion.Equality(nil, nil).CheckPriorityClassEquality(pPriorityClass, &scList.Items[i])
		if updatedPriorityClass != nil {
			atomic.AddUint64(&numMissMatchedPriorityClasses, 1)
			klog.Warningf("spec of priorityClass %v diff in super&tenant master", vPriorityClass.Name)
			if publicPriorityClass(pPriorityClass) {
				c.UpwardController.AddToQueue(clusterName + "/" + pPriorityClass.Name)
			}
		}
	}
}
