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
	"sync"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol/differ"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

const (
	// Default grace period in seconds
	minimumGracePeriodInSeconds = 30
)

var numStatusMissMatchedPods uint64
var numSpecMissMatchedPods uint64
var numUWMetaMissMatchedPods uint64

// StartPatrol starts the period checker for data consistency check. Checker is
// blocking so should be called via a goroutine.
func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stopCh, c.podSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Pod checker")
	}
	c.Patroller.Start(stopCh)
	return nil
}

type Candidate struct {
	cluster  string
	nodeName string
}

func (c *controller) vNodeGCDo() {
	candidates := func() []Candidate {
		c.Lock()
		defer c.Unlock()
		var candidates []Candidate
		for cluster, nodeMap := range c.clusterVNodeGCMap {
			for nodeName, status := range nodeMap {
				if status.Phase == VNodeQuiescing && metav1.Now().After(status.QuiesceStartTime.Add(c.vNodeGCGracePeriod)) {
					c.clusterVNodeGCMap[cluster][nodeName] = VNodeGCStatus{
						QuiesceStartTime: status.QuiesceStartTime,
						Phase:            VNodeDeleting,
					}
					candidates = append(candidates, Candidate{cluster: cluster, nodeName: nodeName})
				} else if status.Phase == VNodeDeleting {
					candidates = append(candidates, Candidate{cluster: cluster, nodeName: nodeName})
				}
			}
		}
		return candidates
	}()

	if len(candidates) == 0 {
		return
	}

	wg := sync.WaitGroup{}
	for _, candidate := range candidates {
		wg.Add(1)
		go func(cluster, nodeName string) {
			defer wg.Done()
			c.deleteClusterVNode(cluster, nodeName)
		}(candidate.cluster, candidate.nodeName)
	}
	wg.Wait()
}

func (c *controller) deleteClusterVNode(cluster, nodeName string) {
	tenantClient, err := c.MultiClusterController.GetClusterClient(cluster)
	if err != nil {
		klog.Infof("cluster is removed, clear clusterVNodeGCMap entry for cluster %s", cluster)
		c.Lock()
		delete(c.clusterVNodeGCMap, cluster)
		c.Unlock()
		return
	}
	opts := metav1.NewDeleteOptions(0)
	opts.PropagationPolicy = &constants.DefaultDeletionPolicy

	tenantClient.CoreV1().Nodes().Delete(context.TODO(), nodeName, *opts)
	// We need to double check here.
	if _, err := tenantClient.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{}); err != nil {
		if !errors.IsNotFound(err) {
			// If we cannot get the state from tenant apiserver, retry
			return
		}
	}

	c.Lock()
	delete(c.clusterVNodeGCMap[cluster], nodeName)
	c.Unlock()
}

// PatrollerDo checks to see if pods in super master informer cache and tenant master
// keep consistency.
func (c *controller) PatrollerDo() {
	clusterNames := c.MultiClusterController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	wg := sync.WaitGroup{}

	numStatusMissMatchedPods = 0
	numSpecMissMatchedPods = 0
	numUWMetaMissMatchedPods = 0

	pList, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing pod from super master informer cache: %v", err)
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
			klog.Errorf("error listing pod from cluster %s informer cache: %v", cluster, err)
			knownClusterSet.Insert(cluster)
			continue
		}
		vList := listObj.(*v1.PodList)
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
		vPod := vObj.Object.(*v1.Pod)

		// pPod not found and vPod is under deletion, we need to delete vPod manually
		if vPod.DeletionTimestamp != nil {
			// since pPod not found in super master, we can force delete vPod
			c.forceDeleteVPod(vObj.GetOwnerCluster(), vPod, false)
			return
		}
		// pPod not found and vPod still exists, the pPod may be deleted manually or by controller pod eviction.
		// If the vPod has not been bound yet, we can create pPod again.
		// If the vPod has been bound, we'd better delete the vPod since the new pPod may have a different nodename.
		if isPodScheduled(vPod) {
			c.forceDeleteVPod(vObj.GetOwnerCluster(), vPod, false)
			metrics.CheckerRemedyStats.WithLabelValues("DeletedTenantPodsDueToSuperEviction").Inc()
			return
		}
		c.requeuePod(vObj.GetOwnerCluster(), vPod)
	}
	d.UpdateFunc = func(vObj, pObj differ.ClusterObject) {
		vPod := vObj.Object.(*v1.Pod)
		pPod := pObj.Object.(*v1.Pod)

		if vPod.DeletionTimestamp != nil && pPod.DeletionTimestamp == nil {
			c.requeuePod(vObj.GetOwnerCluster(), vPod)
			return
		}

		if pPod.Annotations[constants.LabelUID] != string(vPod.UID) {
			if pPod.DeletionTimestamp != nil {
				// pPod is under deletion, waiting for UWS bock populate the pod status.
				return
			}
			klog.Errorf("Found pPod %s delegated UID is different from tenant object", pObj.Key)
			c.graceDeletePPod(pPod)
			return
		}

		if pPod.Spec.NodeName != "" && vPod.Spec.NodeName != "" && pPod.Spec.NodeName != vPod.Spec.NodeName {
			// If pPod can be deleted arbitrarily, e.g., evicted by node controller, this inconsistency may happen.
			// For example, if pPod is deleted just before uws tries to bind the vPod and dws gets a request from checker or
			// user update at the same time, a new pPod is going to be created potentially in a different node.
			// However, uws bound vPod to a wrong node already. There is no easy remediation besides deleting tenant pod.
			c.forceDeleteVPod(vObj.GetOwnerCluster(), vPod, true)
			klog.Errorf("Found pPod %s nodename is different from tenant pod nodename, delete the vPod", pObj.Key)
			metrics.CheckerRemedyStats.WithLabelValues("DeletedTenantPodsDueToNodeMissMatch").Inc()
			return
		}

		clusterName := vObj.GetOwnerCluster()
		vc, err := util.GetVirtualClusterObject(c.MultiClusterController, clusterName)
		if err != nil {
			klog.Errorf("fail to get cluster spec %s: %v", clusterName, err)
			return
		}

		if conversion.Equality(c.Config, vc).CheckPodEquality(pPod, vPod) != nil {
			atomic.AddUint64(&numSpecMissMatchedPods, 1)
			klog.Warningf("spec of pod %s diff in super&tenant master", pObj.Key)
			if err := c.MultiClusterController.RequeueObject(clusterName, vPod); err != nil {
				klog.Errorf("error requeue vPod %s: %v", vObj.Key, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantPods").Inc()
			}
		}

		if conversion.CheckDWPodConditionEquality(pPod, vPod) != nil {
			atomic.AddUint64(&numSpecMissMatchedPods, 1)
			klog.Warningf("DWStatus of pod %s diff in super&tenant master", pObj.Key)
			if err := c.MultiClusterController.RequeueObject(clusterName, vPod); err != nil {
				klog.Errorf("error requeue vpod %v/%v in cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantPods").Inc()
			}
		}

		if conversion.Equality(c.Config, nil).CheckUWPodStatusEquality(pPod, vPod) != nil {
			atomic.AddUint64(&numStatusMissMatchedPods, 1)
			klog.Warningf("status of pod %v/%v diff in super&tenant master", pPod.Namespace, pPod.Name)
			if assignedPod(pPod) {
				c.enqueuePod(pPod)
			}
		}

		if conversion.Equality(c.Config, vc).CheckUWObjectMetaEquality(&pPod.ObjectMeta, &vPod.ObjectMeta) != nil {
			atomic.AddUint64(&numUWMetaMissMatchedPods, 1)
			klog.Warningf("UWObjectMeta of pod %v/%v diff in super&tenant master", vPod.Namespace, vPod.Name)
			if assignedPod(pPod) {
				c.enqueuePod(pPod)
			}
		}
	}
	d.DeleteFunc = func(pObj differ.ClusterObject) {
		c.graceDeletePPod(pObj.Object.(*v1.Pod))
	}

	vSet.Difference(pSet, differ.FilteringHandler{
		Handler: d,
		FilterFunc: func(obj differ.ClusterObject) bool {
			// vObj
			if obj.GetOwnerCluster() != "" {
				vPod := obj.Object.(*v1.Pod)
				if vPod.Spec.NodeName != "" && !isPodScheduled(vPod) {
					// We should skip pods with NodeName set in the spec when unscheduled
					return false
				}
				// Ensure the ClusterVNodePodMap is consistent
				if vPod.Spec.NodeName != "" && !c.checkClusterVNodePodMap(obj.GetOwnerCluster(), vPod.Spec.NodeName, string(vPod.UID)) {
					klog.Errorf("Found vPod %s/%s in cluster %s is missing in ClusterVNodePodMap, added back!", vPod.Namespace, vPod.Name, obj.GetOwnerCluster())
					c.updateClusterVNodePodMap(obj.GetOwnerCluster(), vPod.Spec.NodeName, string(vPod.UID), reconciler.UpdateEvent)
				}
			}
			return differ.DefaultDifferFilter(knownClusterSet)(obj)
		},
	})

	metrics.CheckerMissMatchStats.WithLabelValues("StatusMissMatchedPods").Set(float64(numStatusMissMatchedPods))
	metrics.CheckerMissMatchStats.WithLabelValues("SpecMissMatchedPods").Set(float64(numSpecMissMatchedPods))
	metrics.CheckerMissMatchStats.WithLabelValues("UWMetaMissMatchedPods").Set(float64(numUWMetaMissMatchedPods))

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkNodesOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	// GC unused(orphan) vNodes in tenant masters
	c.vNodeGCDo()
}

func (c *controller) forceDeleteVPod(clusterName string, vPod *v1.Pod, graceful bool) {
	client, err := c.MultiClusterController.GetClusterClient(clusterName)
	if err != nil {
		klog.Errorf("error getting cluster %s clientset: %v", clusterName, err)
		return
	}
	var deleteOptions *metav1.DeleteOptions
	if graceful {
		gracePeriod := int64(minimumGracePeriodInSeconds)
		if vPod.Spec.TerminationGracePeriodSeconds != nil {
			gracePeriod = *vPod.Spec.TerminationGracePeriodSeconds
		}
		deleteOptions = metav1.NewDeleteOptions(gracePeriod)
	} else {
		deleteOptions = metav1.NewDeleteOptions(0)
	}
	deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(vPod.UID))
	if err = client.CoreV1().Pods(vPod.Namespace).Delete(context.TODO(), vPod.Name, *deleteOptions); err != nil {
		klog.Errorf("error deleting pod %v/%v in cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
	} else if vPod.Spec.NodeName != "" {
		c.updateClusterVNodePodMap(clusterName, vPod.Spec.NodeName, string(vPod.UID), reconciler.DeleteEvent)
	}
}

func (c *controller) graceDeletePPod(pPod *v1.Pod) {
	gracePeriod := int64(minimumGracePeriodInSeconds)
	deleteOptions := metav1.NewDeleteOptions(gracePeriod)
	deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pPod.UID))
	if err := c.client.Pods(pPod.Namespace).Delete(context.TODO(), pPod.Name, *deleteOptions); err != nil {
		klog.Errorf("error deleting pPod %v/%v in super master: %v", pPod.Namespace, pPod.Name, err)
	} else {
		metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterPods").Inc()
	}
}

func (c *controller) requeuePod(clusterName string, vPod *v1.Pod) {
	if err := c.MultiClusterController.RequeueObject(clusterName, vPod); err != nil {
		klog.Errorf("error requeue vPod %s/%s in cluster %s: %v", vPod.GetNamespace(), vPod.GetName(), clusterName, err)
	} else {
		metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantPods").Inc()
	}
}

// checkNodesOfTenantCluster checks if any orphan vNode is missed in c.clusterVNodeGCMap, which can happen if syncer
// is restarted.
// Note that this method can be expensive since it cannot leverage pod mccontroller informer cache. The List query
// goes to tenant master directly. If this method causes performance issue, we should consider moving it to another
// periodic thread with a larger check interval.
func (c *controller) checkNodesOfTenantCluster(clusterName string) {
	listObj, err := c.MultiClusterController.ListByObjectType(clusterName, &v1.Node{})
	if err != nil {
		klog.Errorf("failed to list vNode from cluster %s config: %v", clusterName, err)
		return
	}
	nodeList := listObj.(*v1.NodeList)
	for _, vNode := range nodeList.Items {
		if vNode.Labels[constants.LabelVirtualNode] != "true" {
			continue
		}
		func() {
			c.Lock()
			defer c.Unlock()
			if _, exist := c.clusterVNodePodMap[clusterName]; exist {
				if _, exist := c.clusterVNodePodMap[clusterName][vNode.Name]; exist {
					// Active Node
					return
				}
			}
			if _, exist := c.clusterVNodeGCMap[clusterName]; exist {
				if _, exist := c.clusterVNodeGCMap[clusterName][vNode.Name]; exist {
					// In GC list already
					return
				}
			}
			klog.Infof("find an orphan vNode %s missing in GC list in cluster %s", vNode.Name, clusterName)
			c.addToClusterVNodeGCMap(clusterName, vNode.Name)
		}()
	}
}
