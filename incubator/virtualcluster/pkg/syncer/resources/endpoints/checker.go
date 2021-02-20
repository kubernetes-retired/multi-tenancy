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

package endpoints

import (
	"fmt"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol/differ"
)

var numMissingEndPoints uint64
var numMissMatchedEndPoints uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stopCh, c.endpointsSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Endpoint checker")
	}
	c.Patroller.Start(stopCh)
	return nil
}

// PatrollerDo checks to see if Endpoints in super master informer cache and tenant master
// keep consistency.
// Note that eps are managed by tenant/super ep controller separately. The checker will not do GC but only report diff.
func (c *controller) PatrollerDo() {
	clusterNames := c.MultiClusterController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	numMissingEndPoints = 0
	numMissMatchedEndPoints = 0

	pList, err := c.endpointsLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing endpoints from super master informer cache: %v", err)
		return
	}
	pSet := differ.NewDiffSet()
	for _, p := range pList {
		pSet.Insert(differ.ClusterObject{Object: p, Key: differ.DefaultClusterObjectKey(p, "")})
	}

	blockedClusterSet := sets.NewString()
	vSet := differ.NewDiffSet()
	for _, cluster := range clusterNames {
		listObj, err := c.MultiClusterController.List(cluster)
		if err != nil {
			klog.Errorf("error listing endpoints from cluster %s informer cache: %v", cluster, err)
			blockedClusterSet.Insert(cluster)
			continue
		}
		vList := listObj.(*v1.EndpointsList)
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
		// pEp not found and vEp still exists, report the inconsistent ep controller behavior
		klog.Errorf("Cannot find pEp for vEp %s in cluster %s", vObj.Key, vObj.OwnerCluster)
		atomic.AddUint64(&numMissingEndPoints, 1)
	}
	d.UpdateFunc = func(vObj, pObj differ.ClusterObject) {
		v := vObj.Object.(*v1.Endpoints)
		p := pObj.Object.(*v1.Endpoints)
		updated := conversion.Equality(c.Config, nil).CheckEndpointsEquality(p, v)
		if updated != nil {
			atomic.AddUint64(&numMissMatchedEndPoints, 1)
			klog.Warningf("Endpoint %s diff in super&tenant master", pObj.Key)
		}
	}

	vSet.Difference(pSet, differ.FilteringHandler{
		Handler:    d,
		FilterFunc: differ.DefaultDifferFilter(blockedClusterSet),
	})

	metrics.CheckerMissMatchStats.WithLabelValues("MissingEndPoints").Set(float64(numMissingEndPoints))
	metrics.CheckerMissMatchStats.WithLabelValues("MissMatchedEndPoints").Set(float64(numMissMatchedEndPoints))
}
