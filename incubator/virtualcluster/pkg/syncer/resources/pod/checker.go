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

package pod

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

const (
	// Default grace period in seconds
	minimumGracePeriodInSeconds = 30
)

var numStatusMissMatchedPods uint64
var numSpecMissMatchedPods uint64
var numUWMetaMissMatchedPods uint64

// StartPeriodChecker starts the period checker for data consistency check. Checker is
// blocking so should be called via a goroutine.
func (c *controller) StartPeriodChecker(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()

	if !cache.WaitForCacheSync(stopCh, c.podSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Pod checker")
	}

	// Start a loop to do periodic GC of unused(orphan) vNodes in tenant masters.
	go c.vNodeGCWorker(stopCh)

	// Start a loop to periodically check if pods keep consistency between super
	// master and tenant masters.
	wait.Until(c.checkPods, c.periodCheckerPeriod, stopCh)

	return nil
}

func (c *controller) vNodeGCWorker(stopCh <-chan struct{}) {
	klog.Infof("Start VNode GarbageCollector")
	wait.Until(c.vNodeGCDo, c.periodCheckerPeriod, stopCh)
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
				if status.Phase == VNodeQuiescing && metav1.Now().After(status.QuiesceStartTime.Add(constants.DefaultvNodeGCGracePeriod)) {
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
	tenantClient, err := c.multiClusterPodController.GetClusterClient(cluster)
	if err != nil {
		klog.Infof("cluster is removed, clear clusterVNodeGCMap entry for cluster %s", cluster)
		c.Lock()
		delete(c.clusterVNodeGCMap, cluster)
		c.Unlock()
		return
	}
	opts := metav1.NewDeleteOptions(0)
	opts.PropagationPolicy = &constants.DefaultDeletionPolicy

	tenantClient.CoreV1().Nodes().Delete(nodeName, opts)
	// We need to double check here.
	if _, err := tenantClient.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{}); err != nil {
		if !errors.IsNotFound(err) {
			// If we cannot get the state from tenant apiserver, retry
			return
		}
	}
	c.Lock()
	delete(c.clusterVNodeGCMap[cluster], nodeName)
	c.Unlock()
}

// checkPods checks to see if pods in super master informer cache and tenant master
// keep consistency.
func (c *controller) checkPods() {
	clusterNames := c.multiClusterPodController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}
	defer metrics.RecordCheckerScanDuration("pod", time.Now())
	wg := sync.WaitGroup{}

	numStatusMissMatchedPods = 0
	numSpecMissMatchedPods = 0
	numUWMetaMissMatchedPods = 0

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkPodsOfTenantCluster(clusterName)
			c.checkNodesOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pPods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing pods from super master informer cache: %v", err)
		return
	}

	for _, pPod := range pPods {
		clusterName, vNamespace := conversion.GetVirtualOwner(pPod)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}

		shouldDelete := false
		vPodObj, err := c.multiClusterPodController.Get(clusterName, vNamespace, pPod.Name)
		if errors.IsNotFound(err) && pPod.DeletionTimestamp == nil {
			shouldDelete = true
		}
		if err == nil {
			vPod := vPodObj.(*v1.Pod)
			if pPod.Annotations[constants.LabelUID] != string(vPod.UID) {
				shouldDelete = true
				klog.Warningf("Found pPod %s/%s delegated UID is different from tenant object.", pPod.Namespace, pPod.Name)
			} else {
				if !equality.Semantic.DeepEqual(vPod.Status, pPod.Status) {
					numStatusMissMatchedPods++
					klog.Warningf("status of pod %v/%v diff in super&tenant master", pPod.Namespace, pPod.Name)
					if assignedPod(pPod) {
						c.enqueuePod(pPod)
					}
				}
			}
		}
		if shouldDelete {
			if pPod.DeletionTimestamp != nil {
				// pPod is under deletion, waiting for UWS bock populate the pod status.
				continue
			}
			// vPod not found and pPod not under deletion, we need to delete pPod manually
			gracePeriod := int64(minimumGracePeriodInSeconds)
			if pPod.Spec.TerminationGracePeriodSeconds != nil {
				gracePeriod = *pPod.Spec.TerminationGracePeriodSeconds
			}
			deleteOptions := metav1.NewDeleteOptions(gracePeriod)
			deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pPod.UID))
			if err = c.client.Pods(pPod.Namespace).Delete(pPod.Name, deleteOptions); err != nil {
				klog.Errorf("error deleting pPod %v/%v in super master: %v", pPod.Namespace, pPod.Name, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("numDeletedOrphanSuperMasterPods").Inc()
			}
		}
	}

	metrics.CheckerMissMatchStats.WithLabelValues("numStatusMissMatchedPods").Set(float64(numStatusMissMatchedPods))
	metrics.CheckerMissMatchStats.WithLabelValues("numSpecMissMatchedPods").Set(float64(numSpecMissMatchedPods))
	metrics.CheckerMissMatchStats.WithLabelValues("numUWMetaMissMatchedPods").Set(float64(numUWMetaMissMatchedPods))
}

func (c *controller) forceDeletevPod(clusterName string, vPod *v1.Pod, graceful bool) {
	client, err := c.multiClusterPodController.GetClusterClient(clusterName)
	if err != nil {
		klog.Errorf("error getting cluster %s clientset: %v", clusterName, err)
	} else {
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
		if err = client.CoreV1().Pods(vPod.Namespace).Delete(vPod.Name, deleteOptions); err != nil {
			klog.Errorf("error deleting pod %v/%v in cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
		} else if vPod.Spec.NodeName != "" {
			c.updateClusterVNodePodMap(clusterName, vPod.Spec.NodeName, string(vPod.UID), reconciler.DeleteEvent)
		}
	}
}

// checkPodsOfTenantCluster checks to see if pods in specific cluster keeps consistency.
func (c *controller) checkPodsOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterPodController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing pods from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.V(4).Infof("check pods consistency in cluster %s", clusterName)
	podList := listObj.(*v1.PodList)
	for i, vPod := range podList.Items {
		if vPod.Spec.NodeName != "" && !isPodScheduled(&vPod) {
			// We should skip pods with NodeName set in the spec
			continue
		}
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vPod.Namespace)
		pPod, err := c.podLister.Pods(targetNamespace).Get(vPod.Name)
		if errors.IsNotFound(err) {
			// pPod not found and vPod is under deletion, we need to delete vPod manually
			if vPod.DeletionTimestamp != nil {
				// since pPod not found in super master, we can force delete vPod
				c.forceDeletevPod(clusterName, &vPod, false)
			} else {
				// pPod not found and vPod still exists, the pPod may be deleted manually or by controller pod eviction.
				// If the vPod has not been bound yet, we can create pPod again.
				// If the vPod has been bound, we'd better delete the vPod since the new pPod may have a different nodename.
				if isPodScheduled(&vPod) {
					c.forceDeletevPod(clusterName, &vPod, false)
					metrics.CheckerRemedyStats.WithLabelValues("numDeletedTenantPodsDueToSuperEviction").Inc()
				} else {
					if err := c.multiClusterPodController.RequeueObject(clusterName, &podList.Items[i]); err != nil {
						klog.Errorf("error requeue vpod %v/%v in cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
					} else {
						metrics.CheckerRemedyStats.WithLabelValues("numRequeuedTenantPods").Inc()
					}
				}
			}
			continue
		}

		if err != nil {
			klog.Errorf("error getting pPod %s/%s from super master cache: %v", targetNamespace, vPod.Name, err)
			continue
		}
		if pPod.Annotations[constants.LabelUID] != string(vPod.UID) {
			klog.Errorf("Found pPod %s/%s delegated UID is different from tenant object.", targetNamespace, pPod.Name)
			continue
		}
		if pPod.Spec.NodeName != "" && vPod.Spec.NodeName != "" && pPod.Spec.NodeName != vPod.Spec.NodeName {
			// If pPod can be deleted arbitrarily, e.g., evicted by node controller, this inconsistency may happen.
			// For example, if pPod is deleted just before uws tries to bind the vPod and dws gets a request from checker or
			// user update at the same time, a new pPod is going to be created potentially in a differnt node.
			// However, uws bound vPod to a wrong node already. There is no easy remediation besides deleting tenant pod.
			c.forceDeletevPod(clusterName, &vPod, true)
			klog.Errorf("Found pPod %s/%s nodename is different from tenant pod nodename, delete the vPod.", targetNamespace, pPod.Name)
			metrics.CheckerRemedyStats.WithLabelValues("numDeletedTenantPodsDueToNodeMissMatch").Inc()
			continue
		}
		spec, err := c.multiClusterPodController.GetSpec(clusterName)
		if err != nil {
			klog.Errorf("fail to get cluster spec : %s", clusterName)
			continue
		}
		updatedPod := conversion.Equality(spec).CheckPodEquality(pPod, &podList.Items[i])
		if updatedPod != nil {
			atomic.AddUint64(&numSpecMissMatchedPods, 1)
			klog.Warningf("spec of pod %v/%v diff in super&tenant master", vPod.Namespace, vPod.Name)
			if err := c.multiClusterPodController.RequeueObject(clusterName, &podList.Items[i]); err != nil {
				klog.Errorf("error requeue vpod %v/%v in cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("numRequeuedTenantPods").Inc()
			}
		}

		updatedMeta := conversion.Equality(spec).CheckUWObjectMetaEquality(&pPod.ObjectMeta, &podList.Items[i].ObjectMeta)
		if updatedMeta != nil {
			atomic.AddUint64(&numUWMetaMissMatchedPods, 1)
			klog.Warningf("UWObjectMeta of pod %v/%v diff in super&tenant master", vPod.Namespace, vPod.Name)
			if assignedPod(pPod) {
				c.enqueuePod(pPod)
			}
		}
	}
}

// checkNodesOfTenantCluster checks if any orphan vNode is missed in c.clusterVNodeGCMap, which can happen if syncer
// is restarted.
// Note that this method can be expensive since it cannot leverage pod mccontroller informer cache. The List query
// goes to tenant master directly. If this method causes performance issue, we should consider moving it to another
// periodic thread with a larger check interval.
func (c *controller) checkNodesOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterPodController.ListByObjectType(clusterName, &v1.Node{})
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
