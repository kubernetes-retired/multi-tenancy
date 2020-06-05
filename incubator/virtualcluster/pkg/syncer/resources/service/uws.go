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

package service

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
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
		return fmt.Errorf("could not find pService %s/%s's vService in controller cache %v", vNamespace, pName, err)
	}
	vService := vServiceObj.(*v1.Service)
	if pService.Annotations[constants.LabelUID] != string(vService.UID) {
		return fmt.Errorf("BackPopulated pService %s/%s delegated UID is different from updated object.", pService.Namespace, pService.Name)
	}

	tenantClient, err := c.multiClusterServiceController.GetClusterClient(clusterName)
	if err != nil {
		return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
	}

	if !equality.Semantic.DeepEqual(vService.Status, pService.Status) {
		newService := vService.DeepCopy()
		newService.Status = pService.Status
		if _, err = tenantClient.CoreV1().Services(vService.Namespace).UpdateStatus(newService); err != nil {
			return fmt.Errorf("failed to back populate service %s/%s status update for cluster %s: %v", vService.Namespace, vService.Name, clusterName, err)
		}
	}
	return nil
}
