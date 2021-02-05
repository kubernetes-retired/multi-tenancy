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

	pkgerr "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.podSynced, c.serviceSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.UpwardController.Start(stopCh)
}

func (c *controller) BackPopulate(key string) error {
	pNamespace, pName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key %v: %v", key, err))
		return nil
	}

	pPod, err := c.podLister.Pods(pNamespace).Get(pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	clusterName, vNamespace := conversion.GetVirtualOwner(pPod)
	if clusterName == "" || vNamespace == "" {
		klog.Infof("drop pod %s/%s which is not belongs to any tenant", pNamespace, pName)
		return nil
	}

	vPodObj, err := c.MultiClusterController.Get(clusterName, vNamespace, pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return pkgerr.Wrapf(err, "could not find pPod %s/%s's vPod in controller cache", vNamespace, pName)
	}
	vPod := vPodObj.(*v1.Pod)
	if pPod.Annotations[constants.LabelUID] != string(vPod.UID) {
		return fmt.Errorf("BackPopulated pPod %s/%s delegated UID is different from updated object.", pPod.Namespace, pPod.Name)
	}

	tenantClient, err := c.MultiClusterController.GetClusterClient(clusterName)
	if err != nil {
		return pkgerr.Wrapf(err, "failed to create client from cluster %s config", clusterName)
	}

	// If tenant Pod has not been assigned, bind to virtual Node.
	if vPod.Spec.NodeName == "" {
		n, err := c.client.Nodes().Get(context.TODO(), pPod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get node %s from super master: %v", pPod.Spec.NodeName, err)
		}
		// We need to handle the race with vNodeGC thread here.
		if err = func() error {
			c.Lock()
			defer c.Unlock()
			if !c.removeQuiescingNodeFromClusterVNodeGCMap(clusterName, pPod.Spec.NodeName) {
				return fmt.Errorf("The bind target vNode %s is being GCed in cluster %s, retry", pPod.Spec.NodeName, clusterName)
			}
			return nil
		}(); err != nil {
			return err
		}

		if _, err := c.MultiClusterController.GetByObjectType(clusterName, "", n.GetName(), &v1.Node{}); err != nil {
			// check if target node has already registered on the vc
			// before creating
			if !errors.IsNotFound(err) {
				return err
			}
			vn, err := vnode.NewVirtualNode(c.vnodeProvider, n)
			if err != nil {
				return fmt.Errorf("failed to create virtual node %s in cluster %s from provider: %v", pPod.Spec.NodeName, clusterName, err)
			}
			_, err = tenantClient.CoreV1().Nodes().Create(context.TODO(), vn, metav1.CreateOptions{})
			if err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create virtual node %s in cluster %s with err: %v", pPod.Spec.NodeName, clusterName, err)
			}
		}

		err = tenantClient.CoreV1().Pods(vPod.Namespace).Bind(context.TODO(), &v1.Binding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vPod.Name,
				Namespace: vPod.Namespace,
			},
			Target: v1.ObjectReference{
				Kind:       "Node",
				Name:       pPod.Spec.NodeName,
				APIVersion: "v1",
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to bind vPod %s/%s to node %s %v", vPod.Namespace, vPod.Name, pPod.Spec.NodeName, err)
		}
		// virtual pod has been updated, refetch the latest version
		if vPod, err = tenantClient.CoreV1().Pods(vPod.Namespace).Get(context.TODO(), vPod.Name, metav1.GetOptions{}); err != nil {
			return fmt.Errorf("failed to retrieve vPod %s/%s from cluster %s: %v", vNamespace, pName, clusterName, err)
		}
	} else {
		// Check if the vNode exists in Tenant master.
		if _, err := c.MultiClusterController.GetByObjectType(clusterName, "", vPod.Spec.NodeName, &v1.Node{}); err != nil {
			if errors.IsNotFound(err) {
				// We have consistency issue here, do not fix for now. TODO: add to metrics
			}
			return fmt.Errorf("failed to check vNode %s of vPod %s in cluster %s: %v ", vPod.Spec.NodeName, vPod.Name, clusterName, err)
		}
	}

	vc, err := util.GetVirtualClusterObject(c.MultiClusterController, clusterName)
	if err != nil {
		return err
	}

	var newPod *v1.Pod
	updatedMeta := conversion.Equality(c.Config, vc).CheckUWObjectMetaEquality(&pPod.ObjectMeta, &vPod.ObjectMeta)
	if updatedMeta != nil {
		newPod = vPod.DeepCopy()
		newPod.ObjectMeta = *updatedMeta
		if _, err = tenantClient.CoreV1().Pods(vPod.Namespace).Update(context.TODO(), newPod, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to back populate pod %s/%s meta update for cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
		}
	}

	if newStatus := conversion.Equality(c.Config, vc).CheckUWPodStatusEquality(pPod, vPod); newStatus != nil {
		if newPod == nil {
			newPod = vPod.DeepCopy()
		} else {
			// Pod has been updated, let us fetch the latest version.
			if newPod, err = tenantClient.CoreV1().Pods(vPod.Namespace).Get(context.TODO(), vPod.Name, metav1.GetOptions{}); err != nil {
				return fmt.Errorf("failed to retrieve vPod %s/%s from cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
			}
		}
		newPod.Status = *newStatus
		if _, err = tenantClient.CoreV1().Pods(vPod.Namespace).UpdateStatus(context.TODO(), newPod, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to back populate pod %s/%s status update for cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
		}
	}

	// pPod is under deletion.
	if pPod.DeletionTimestamp != nil {
		if vPod.DeletionTimestamp == nil {
			klog.V(4).Infof("pPod %s/%s is under deletion accidentally", pPod.Namespace, pPod.Name)
			gracePeriod := int64(minimumGracePeriodInSeconds)
			if vPod.Spec.TerminationGracePeriodSeconds != nil {
				gracePeriod = *vPod.Spec.TerminationGracePeriodSeconds
			}
			deleteOptions := metav1.NewDeleteOptions(gracePeriod)
			if err = tenantClient.CoreV1().Pods(vPod.Namespace).Delete(context.TODO(), vPod.Name, *deleteOptions); err != nil {
				return err
			}
		} else if *vPod.DeletionGracePeriodSeconds != *pPod.DeletionGracePeriodSeconds {
			klog.V(4).Infof("delete virtual pPod %s/%s with grace period seconds %v", vPod.Namespace, vPod.Name, *pPod.DeletionGracePeriodSeconds)
			deleteOptions := metav1.NewDeleteOptions(*pPod.DeletionGracePeriodSeconds)
			deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(vPod.UID))
			if err = tenantClient.CoreV1().Pods(vPod.Namespace).Delete(context.TODO(), vPod.Name, *deleteOptions); err != nil {
				return err
			}
			if vPod.Spec.NodeName != "" {
				c.updateClusterVNodePodMap(clusterName, vPod.Spec.NodeName, string(vPod.UID), reconciler.DeleteEvent)
			}
		}
	}

	return nil
}
