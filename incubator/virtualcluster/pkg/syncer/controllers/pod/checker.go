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
	"context"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"

	ctrl "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
)

// StartPeriodChecker starts the period checker for data consistency check. Checker is
// blocking so should be called via a goroutine.
func (c *controller) StartPeriodChecker(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()

	// Start a loop to periodically check if pods keep consistency between super
	// master and tenant masters.
	wait.Until(c.checkPods, c.periodCheckerPeriod, stopCh)
}

// checkPods checks to see if pods in super master informer cache and tenant master
// keep consistency.
func (c *controller) checkPods() {
	clusterNames := c.multiClusterPodController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	for _, clusterName := range clusterNames {
		cluster := c.multiClusterPodController.GetCluster(clusterName)
		if cluster == nil {
			klog.Errorf("failed to locate cluster %s", clusterName)
			continue
		}

		c.checkPodsOfCluster(clusterName, cluster)
	}

	pPods, err := c.podLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing pods from super master informer cache: %v", err)
		return
	}

	for _, pPod := range pPods {
		clusterName, vNamespace := conversion.GetOwner(pPod)
		vPodObj, err := c.multiClusterPodController.Get(clusterName, vNamespace, pPod.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				if pPod.DeletionTimestamp != nil {
					// pPod is under deletion, waiting for UWS bock populate the pod status.
					continue
				}
				if err = c.client.Pods(pPod.Namespace).Delete(pPod.Name, &metav1.DeleteOptions{}); err != nil {
					klog.Errorf("error deleting pPod %s/%s in super master: %v", pPod.Namespace, pPod.Name, err)
					continue
				}
			}
		}
		vPod := vPodObj.(*v1.Pod)

		// pod has been updated by super master
		if !equality.Semantic.DeepEqual(vPod.Status, pPod.Status) {
			c.enqueuePod(pPod)
		}
	}
}

// checkPodsOfCluster checks to see if pods in specific cluster keeps consistency.
func (c *controller) checkPodsOfCluster(clusterName string, cluster ctrl.ClusterInterface) {
	clusterInformerCache, err := cluster.GetCache()
	if err != nil {
		klog.Errorf("failed to get informer cache for cluster %s", clusterName)
		return
	}
	podList := &v1.PodList{}
	err = clusterInformerCache.List(context.TODO(), podList)
	if err != nil {
		klog.Errorf("error listing pods from cluster informer cache: %v", err)
		return
	}

	klog.Infof("check pods consistency in cluster %s", clusterName)
	for _, vPod := range podList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vPod.Namespace)
		pPod, err := c.podLister.Pods(targetNamespace).Get(vPod.Name)
		if err == nil {
			updatedPod := conversion.CheckPodEquality(pPod, &vPod)
			if updatedPod != nil {
				klog.Infof("pod %v/%v diff in super&tenant master", vPod.Namespace, vPod.Name)
				pPod, err = c.client.Pods(targetNamespace).Update(updatedPod)
				if err != nil {
					klog.Errorf("error updating pod %v/%v: %v", vPod.Namespace, vPod.Name, err)
					continue
				}
			}
		} else if errors.IsNotFound(err) {
			// pPod not found and vPod is under deletion
			if vPod.DeletionTimestamp != nil {
				clusterClient, err := cluster.GetClient()
				if err != nil {
					klog.Errorf("error getting cluster %s clientset: %v", err)
					continue
				}
				deleteOptions := metav1.NewDeleteOptions(*vPod.DeletionGracePeriodSeconds)
				deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pPod.UID))
				if err = clusterClient.CoreV1().Pods(vPod.Namespace).Delete(vPod.Name, deleteOptions); err != nil {
					klog.Errorf("error deleting pod %v/%v in cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
				}
			}
		}
	}
}
