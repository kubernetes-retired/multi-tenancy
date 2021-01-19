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

	pkgerr "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
)

// StartUWS starts the upward syncer
// and blocks until an empty struct is sent to the stop channel.
func (c *controller) StartUWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.serviceSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.upwardServiceController.Start(stopCh)
}

func (c *controller) BackPopulate(key string) error {
	pNamespace, pName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key %v: %v", key, err))
		return nil
	}

	pService, err := c.serviceLister.Services(pNamespace).Get(pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Make sure the super cluster IP is added to the annotation so that it can be back populated to the tenant object
	if pService.Spec.ClusterIP != "" && pService.Annotations[constants.LabelSuperClusterIP] != pService.Spec.ClusterIP {
		if pService.Annotations == nil {
			pService.Annotations = make(map[string]string)
		}
		pService.Annotations[constants.LabelSuperClusterIP] = pService.Spec.ClusterIP
		_, err = c.serviceClient.Services(pNamespace).Update(context.TODO(), pService, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		// wait for the next reconcile for the rest of backpopulate work.
		return nil
	}

	clusterName, vNamespace := conversion.GetVirtualOwner(pService)
	if clusterName == "" || vNamespace == "" {
		klog.Infof("drop service %s/%s which is not belongs to any tenant", pNamespace, pName)
		return nil
	}

	vServiceObj, err := c.multiClusterServiceController.Get(clusterName, vNamespace, pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return pkgerr.Wrapf(err, "could not find pService %s/%s's vService in controller cache", vNamespace, pName)
	}
	vService := vServiceObj.(*v1.Service)
	if pService.Annotations[constants.LabelUID] != string(vService.UID) {
		return fmt.Errorf("BackPopulated pService %s/%s delegated UID is different from updated object.", pService.Namespace, pService.Name)
	}

	tenantClient, err := c.multiClusterServiceController.GetClusterClient(clusterName)
	if err != nil {
		return pkgerr.Wrapf(err, "failed to create client from cluster %s config", clusterName)
	}

	vc, err := util.GetVirtualClusterObject(c.multiClusterServiceController, clusterName)
	if err != nil {
		return pkgerr.Wrapf(err, "failed to get spec of cluster %s", clusterName)
	}

	var newService *v1.Service
	updatedMeta := conversion.Equality(c.config, vc).CheckUWObjectMetaEquality(&pService.ObjectMeta, &vService.ObjectMeta)
	if updatedMeta != nil {
		newService = vService.DeepCopy()
		newService.ObjectMeta = *updatedMeta
		if _, err = tenantClient.CoreV1().Services(vService.Namespace).Update(context.TODO(), newService, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to back populate service %s/%s meta update for cluster %s: %v", vService.Namespace, vService.Name, clusterName, err)
		}
	}

	if !equality.Semantic.DeepEqual(vService.Status, pService.Status) {
		if newService == nil {
			newService = vService.DeepCopy()
		} else {
			// vService has been updated, let us fetch the lastest version.
			if newService, err = tenantClient.CoreV1().Services(vService.Namespace).Get(context.TODO(), vService.Name, metav1.GetOptions{}); err != nil {
				return fmt.Errorf("failed to retrieve vService %s/%s from cluster %s: %v", vService.Namespace, vService.Name, clusterName, err)
			}
		}
		newService.Status = pService.Status
		if _, err = tenantClient.CoreV1().Services(vService.Namespace).UpdateStatus(context.TODO(), newService, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to back populate service %s/%s status update for cluster %s: %v", vService.Namespace, vService.Name, clusterName, err)
		}
	}
	return nil
}
