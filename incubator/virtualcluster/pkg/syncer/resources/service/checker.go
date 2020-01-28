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
	"sync"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func (c *controller) StartPeriodChecker(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.serviceSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Service checker")
	}

	wait.Until(c.checkServices, c.periodCheckerPeriod, stopCh)
	return nil
}

// checkServices check if services keep consistency between super
// master and tenant masters.
func (c *controller) checkServices() {
	clusterNames := c.multiClusterServiceController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	wg := sync.WaitGroup{}

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkServicesOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pServices, err := c.serviceLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing services from super master informer cache: %v", err)
		return
	}

	for _, pService := range pServices {
		clusterName, vNamespace := conversion.GetVirtualOwner(pService)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}

		_, err := c.multiClusterServiceController.Get(clusterName, vNamespace, pService.Name)
		if errors.IsNotFound(err) {
			deleteOptions := metav1.NewPreconditionDeleteOptions(string(pService.UID))
			if err = c.serviceClient.Services(pService.Namespace).Delete(pService.Name, deleteOptions); err != nil {
				klog.Errorf("error deleting pService %s/%s in super master: %v", pService.Namespace, pService.Name, err)
			}
			continue
		}
	}
}

func (c *controller) checkServicesOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterServiceController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing services from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.Infof("check services consistency in cluster %s", clusterName)
	svcList := listObj.(*v1.ServiceList)
	for i, vService := range svcList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vService.Namespace)
		pService, err := c.serviceLister.Services(targetNamespace).Get(vService.Name)
		if errors.IsNotFound(err) {
			if err := c.multiClusterServiceController.RequeueObject(clusterName, &svcList.Items[i], reconciler.AddEvent); err != nil {
				klog.Errorf("error requeue vservice %v/%v in cluster %s: %v", vService.Namespace, vService.Name, clusterName, err)
			}
			continue
		}

		if err != nil {
			klog.Errorf("failed to get pService %s/%s from super master cache: %v", targetNamespace, vService.Name, err)
			continue
		}

		updatedService := conversion.CheckServiceEquality(pService, &svcList.Items[i])
		if updatedService != nil {
			klog.Warningf("spec of service %v/%v diff in super&tenant master", vService.Namespace, vService.Name)
		}
	}
}
