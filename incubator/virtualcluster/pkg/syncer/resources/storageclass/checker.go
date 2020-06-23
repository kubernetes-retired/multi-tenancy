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

package storageclass

import (
	"fmt"
	"sync"
	"sync/atomic"

	v1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
)

var numMissMatchedStorageClasses uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.storageclassSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service checker")
	}
	c.storageClassPatroller.Start(stopCh)
	return nil
}

// ParollerDo check if StorageClass keeps consistency between super master and tenant masters.
func (c *controller) PatrollerDo() {
	clusterNames := c.multiClusterStorageClassController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up storage class period checker")
		return
	}

	wg := sync.WaitGroup{}
	numMissMatchedStorageClasses = 0

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkStorageClassOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pStorageClassList, err := c.storageclassLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing storageclass from super master informer cache: %v", err)
		return
	}

	for _, pStorageClass := range pStorageClassList {
		if !publicStorageClass(pStorageClass) {
			continue
		}
		for _, clusterName := range clusterNames {
			_, err := c.multiClusterStorageClassController.Get(clusterName, "", pStorageClass.Name)
			if err != nil {
				if errors.IsNotFound(err) {
					metrics.CheckerRemedyStats.WithLabelValues("numRequeuedSuperMasterStorageClasses").Inc()
					c.upwardStorageClassController.AddToQueue(clusterName + "/" + pStorageClass.Name)
				}
				klog.Errorf("fail to get storageclass from cluster %s: %v", clusterName, err)
			}
		}
	}

	metrics.CheckerMissMatchStats.WithLabelValues("numMissMatchedStorageClasses").Set(float64(numMissMatchedStorageClasses))
}

func (c *controller) checkStorageClassOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterStorageClassController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing storageclass from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.V(4).Infof("check storageclass consistency in cluster %s", clusterName)
	scList := listObj.(*v1.StorageClassList)
	for i, vStorageClass := range scList.Items {
		pStorageClass, err := c.storageclassLister.Get(vStorageClass.Name)
		if errors.IsNotFound(err) {
			// super master is the source of the truth for sc object, delete tenant master obj
			tenantClient, err := c.multiClusterStorageClassController.GetClusterClient(clusterName)
			if err != nil {
				klog.Errorf("error getting cluster %s clientset: %v", clusterName, err)
				continue
			}
			opts := &metav1.DeleteOptions{
				PropagationPolicy: &constants.DefaultDeletionPolicy,
			}
			if err := tenantClient.StorageV1().StorageClasses().Delete(vStorageClass.Name, opts); err != nil {
				klog.Errorf("error deleting storageclass %v in cluster %s: %v", vStorageClass.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("numDeletedOrphanTenantStorageClasses").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("failed to get pStorageClass %s from super master cache: %v", vStorageClass.Name, err)
			continue
		}

		updatedStorageClass := conversion.Equality(nil, nil).CheckStorageClassEquality(pStorageClass, &scList.Items[i])
		if updatedStorageClass != nil {
			atomic.AddUint64(&numMissMatchedStorageClasses, 1)
			klog.Warningf("spec of storageClass %v diff in super&tenant master", vStorageClass.Name)
			if publicStorageClass(pStorageClass) {
				c.upwardStorageClassController.AddToQueue(clusterName + "/" + pStorageClass.Name)
			}
		}
	}
}
