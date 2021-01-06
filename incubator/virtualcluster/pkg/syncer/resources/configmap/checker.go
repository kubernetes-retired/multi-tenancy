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
	"context"
	"fmt"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol/differ"
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

	pConfigMaps, err := c.configMapLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing configmaps from super master informer cache: %v", err)
		return
	}
	pSet := differ.NewDiffSet()
	for _, pCM := range pConfigMaps {
		pSet.Insert(differ.ClusterObject{Object: pCM, Key: differ.DefaultClusterObjectKey(pCM, "")})
	}

	vSet := differ.NewDiffSet()
	for _, cluster := range clusterNames {
		listObj, err := c.multiClusterConfigMapController.List(cluster)
		if err != nil {
			klog.Errorf("error listing configmaps from cluster %s informer cache: %v", cluster, err)
			continue
		}
		cmList := listObj.(*v1.ConfigMapList)
		for i := range cmList.Items {
			vSet.Insert(differ.ClusterObject{
				Object:       &cmList.Items[i],
				OwnerCluster: cluster,
				Key:          differ.DefaultClusterObjectKey(&cmList.Items[i], cluster),
			})
		}
	}

	configMapDiffer := differ.HandlerFuncs{}
	configMapDiffer.AddFunc = func(vObj differ.ClusterObject) {
		if err := c.multiClusterConfigMapController.RequeueObject(vObj.OwnerCluster, vObj.Object); err != nil {
			klog.Errorf("error requeue vConfigMap %v/%v in cluster %s: %v", vObj.GetNamespace(), vObj.GetName(), vObj.GetOwnerCluster(), err)
		} else {
			metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantConfigMaps").Inc()
		}
	}
	configMapDiffer.UpdateFunc = func(vObj, pObj differ.ClusterObject) {
		vCM := vObj.Object.(*v1.ConfigMap)
		pCM := pObj.Object.(*v1.ConfigMap)

		if pCM.Annotations[constants.LabelUID] != string(vCM.UID) {
			klog.Errorf("Found pConfigMap %s delegated UID is different from tenant object.", pObj.Key)
			configMapDiffer.OnDelete(pObj)
			return
		}
		spec, err := c.multiClusterConfigMapController.GetSpec(vObj.GetOwnerCluster())
		if err != nil {
			klog.Errorf("fail to get cluster spec : %s", vObj.GetOwnerCluster())
			return
		}
		updated := conversion.Equality(c.config, spec).CheckConfigMapEquality(pCM, vCM)
		if updated != nil {
			atomic.AddUint64(&numMissMatchedConfigMaps, 1)
			klog.Warningf("ConfigMap %s diff in super&tenant master", pObj.Key)
		}
	}
	configMapDiffer.DeleteFunc = func(pObj differ.ClusterObject) {
		deleteOptions := &metav1.DeleteOptions{}
		deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pObj.GetUID()))
		if err = c.configMapClient.ConfigMaps(pObj.GetNamespace()).Delete(context.TODO(), pObj.GetName(), *deleteOptions); err != nil {
			klog.Errorf("error deleting pConfigMap %s in super master: %v", pObj.Key, err)
		} else {
			metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterConfigMaps").Inc()
		}
	}

	vSet.Difference(pSet, differ.FilteringHandler{
		Handler:    configMapDiffer,
		FilterFunc: differ.DefaultDifferFilter,
	})

	metrics.CheckerMissMatchStats.WithLabelValues("MissMatchedConfigMaps").Set(float64(numMissMatchedConfigMaps))
}
