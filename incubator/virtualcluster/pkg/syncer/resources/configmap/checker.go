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

package configmap

import (
	"fmt"
	"sync"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
)

var numMissMatchedConfigMaps uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stopCh, c.configMapSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting ConfigMap checker")
	}
	c.configMapPatroller.Start(stopCh)
	return nil
}

// PatrollerDo checks to see if configmaps in super master informer cache and tenant master
// keep consistency.
func (c *controller) PatrollerDo() {
	clusterNames := c.multiClusterConfigMapController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	wg := sync.WaitGroup{}
	numMissMatchedConfigMaps = 0

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkConfigMapsOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pConfigMaps, err := c.configMapLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing configmaps from super master informer cache: %v", err)
		return
	}

	for _, pConfigMap := range pConfigMaps {
		clusterName, vNamespace := conversion.GetVirtualOwner(pConfigMap)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}
		shouldDelete := false
		vConfigMapObj, err := c.multiClusterConfigMapController.Get(clusterName, vNamespace, pConfigMap.Name)
		if errors.IsNotFound(err) {
			shouldDelete = true
		}
		if err == nil {
			vConfigMap := vConfigMapObj.(*v1.ConfigMap)
			if pConfigMap.Annotations[constants.LabelUID] != string(vConfigMap.UID) {
				shouldDelete = true
				klog.Warningf("Found pConfigMap %s/%s delegated UID is different from tenant object.", pConfigMap.Namespace, pConfigMap.Name)
			}
		}

		if shouldDelete {
			// vConfigMap not found and pConfigMap still exist, we need to delete pConfigMap manually
			deleteOptions := &metav1.DeleteOptions{}
			deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pConfigMap.UID))
			if err = c.configMapClient.ConfigMaps(pConfigMap.Namespace).Delete(pConfigMap.Name, deleteOptions); err != nil {
				klog.Errorf("error deleting pConfigMap %v/%v in super master: %v", pConfigMap.Namespace, pConfigMap.Name, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterConfigMaps").Inc()
			}
		}
	}

	metrics.CheckerMissMatchStats.WithLabelValues("MissMatchedConfigMaps").Set(float64(numMissMatchedConfigMaps))
}

// checkConfigMapsOfTenantCluster checks to see if configmaps in specific cluster keeps consistency.
func (c *controller) checkConfigMapsOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterConfigMapController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing configmaps from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.V(4).Infof("check configmaps consistency in cluster %s", clusterName)
	configMapList := listObj.(*v1.ConfigMapList)
	for i, vConfigMap := range configMapList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vConfigMap.Namespace)
		pConfigMap, err := c.configMapLister.ConfigMaps(targetNamespace).Get(vConfigMap.Name)
		if errors.IsNotFound(err) {
			// pConfigMap not found and vConfigMap still exists, we need to create pConfigMap again
			if err := c.multiClusterConfigMapController.RequeueObject(clusterName, &configMapList.Items[i]); err != nil {
				klog.Errorf("error requeue vConfigMap %v/%v in cluster %s: %v", vConfigMap.Namespace, vConfigMap.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantConfigMaps").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("error getting pConfigMap %s/%s from super master cache: %v", targetNamespace, vConfigMap.Name, err)
			continue
		}

		if pConfigMap.Annotations[constants.LabelUID] != string(vConfigMap.UID) {
			klog.Errorf("Found pConfigMap %s/%s delegated UID is different from tenant object.", targetNamespace, pConfigMap.Name)
			continue
		}
		spec, err := c.multiClusterConfigMapController.GetSpec(clusterName)
		if err != nil {
			klog.Errorf("fail to get cluster spec : %s", clusterName)
			continue
		}
		updated := conversion.Equality(c.config, spec).CheckConfigMapEquality(pConfigMap, &vConfigMap)
		if updated != nil {
			atomic.AddUint64(&numMissMatchedConfigMaps, 1)
			klog.Warningf("ConfigMap %v/%v diff in super&tenant master", vConfigMap.Namespace, vConfigMap.Name)
		}
	}
}
