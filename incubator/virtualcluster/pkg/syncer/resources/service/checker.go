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

package service

import (
	"context"
	"fmt"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol/differ"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
)

var numSpecMissMatchedServices uint64
var numStatusMissMatchedServices uint64
var numUWMetaMissMatchedServices uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.serviceSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service checker")
	}
	c.Patroller.Start(stopCh)
	return nil
}

// PatrollerDo check if services keep consistency between super
// master and tenant masters.
func (c *controller) PatrollerDo() {
	clusterNames := c.MultiClusterController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	numSpecMissMatchedServices = 0
	numStatusMissMatchedServices = 0
	numUWMetaMissMatchedServices = 0

	pList, err := c.serviceLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing service from super master informer cache: %v", err)
		return
	}
	pSet := differ.NewDiffSet()
	for _, p := range pList {
		pSet.Insert(differ.ClusterObject{Object: p, Key: differ.DefaultClusterObjectKey(p, "")})
	}

	knownClusterSet := sets.NewString(clusterNames...)
	vSet := differ.NewDiffSet()
	for _, cluster := range clusterNames {
		listObj, err := c.MultiClusterController.List(cluster)
		if err != nil {
			klog.Errorf("error listing service from cluster %s informer cache: %v", cluster, err)
			knownClusterSet.Insert(cluster)
			continue
		}
		vList := listObj.(*v1.ServiceList)
		for i := range vList.Items {
			vSet.Insert(differ.ClusterObject{
				Object:       &vList.Items[i],
				OwnerCluster: cluster,
				Key:          differ.DefaultClusterObjectKey(&vList.Items[i], cluster),
			})
		}
	}

	d := differ.HandlerFuncs{}
	d.AddFunc = func(vObj differ.ClusterObject) {
		if err := c.MultiClusterController.RequeueObject(vObj.GetOwnerCluster(), vObj.Object); err != nil {
			klog.Errorf("error requeue vService %s in cluster %s: %v", vObj.Key, vObj.GetOwnerCluster(), err)
		} else {
			metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantServices").Inc()
		}
	}
	d.UpdateFunc = func(vObj, pObj differ.ClusterObject) {
		v := vObj.Object.(*v1.Service)
		p := pObj.Object.(*v1.Service)

		if p.Annotations[constants.LabelUID] != string(v.UID) {
			klog.Warningf("Found pService %s delegated UID is different from tenant object", pObj.Key)
			d.OnDelete(pObj)
			return
		}

		vc, err := util.GetVirtualClusterObject(c.MultiClusterController, vObj.GetOwnerCluster())
		if err != nil {
			klog.Errorf("fail to get cluster spec : %s: %v", vObj.GetOwnerCluster(), err)
			return
		}
		updatedService := conversion.Equality(c.Config, vc).CheckServiceEquality(p, v)
		if updatedService != nil {
			atomic.AddUint64(&numSpecMissMatchedServices, 1)
			klog.Warningf("spec of service %s diff in super&tenant master", pObj.Key)
			d.OnAdd(vObj)
			return
		}

		if isBackPopulateService(p) {
			enqueue := false
			updatedMeta := conversion.Equality(c.Config, vc).CheckUWObjectMetaEquality(&p.ObjectMeta, &v.ObjectMeta)
			if updatedMeta != nil {
				atomic.AddUint64(&numUWMetaMissMatchedServices, 1)
				enqueue = true
				klog.Warningf("UWObjectMeta of service %s diff in super&tenant master", pObj.Key)
			}
			if !equality.Semantic.DeepEqual(p.Status, v.Status) {
				enqueue = true
				atomic.AddUint64(&numStatusMissMatchedServices, 1)
				klog.Warningf("Status of service %s diff in super&tenant master", pObj)
			}
			if enqueue {
				c.enqueueService(p)
			}
		}
	}
	d.DeleteFunc = func(pObj differ.ClusterObject) {
		deleteOptions := metav1.NewPreconditionDeleteOptions(string(pObj.GetUID()))
		if err = c.serviceClient.Services(pObj.GetNamespace()).Delete(context.TODO(), pObj.GetName(), *deleteOptions); err != nil {
			klog.Errorf("error deleting pService %s in super master: %v", pObj.Key, err)
		} else {
			metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterServices").Inc()
		}
	}

	vSet.Difference(pSet, differ.FilteringHandler{
		Handler:    d,
		FilterFunc: differ.DefaultDifferFilter(knownClusterSet),
	})

	metrics.CheckerMissMatchStats.WithLabelValues("SpecMissMatchedServices").Set(float64(numSpecMissMatchedServices))
	metrics.CheckerMissMatchStats.WithLabelValues("StatusMissMatchedServices").Set(float64(numStatusMissMatchedServices))
	metrics.CheckerMissMatchStats.WithLabelValues("UWMetaMissMatchedServices").Set(float64(numUWMetaMissMatchedServices))
}
