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

package crd

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
)

var numMissMatchedCRD uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	if !cache.WaitForCacheSync(stopCh, c.crdSynced, c.vcSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting networkpolicy checker")
	}
	c.crdPatroller.Start(stopCh)
	return nil
}

// PatrollerDo checks to see if annotated CRD is in super master informer cache and then synced to tenant cluster
func (c *controller) PatrollerDo() {
	clusterNames := c.multiClusterCrdController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up CRD period checker")
		return
	}
	wg := sync.WaitGroup{}
	numMissMatchedCRD = 0

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkCRDOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pCRDList := &v1beta1.CustomResourceDefinitionList{}
	err := c.superClient.List(context.Background(), pCRDList)
	if err != nil {
		klog.Errorf("error listing crd from super master informer cache: %v", err)
		return
	}
	for _, pCRD := range pCRDList.Items {
		if !publicCRD(&pCRD) {
			continue
		}
		for _, clusterName := range clusterNames {
			_, err := c.multiClusterCrdController.Get(clusterName, "", pCRD.Name)
			if err != nil {
				if errors.IsNotFound(err) {
					metrics.CheckerRemedyStats.WithLabelValues("RequeuedSuperMasterCRD").Inc()
					klog.Infof("patroller create crd %v in virtual cluster", clusterName+"/"+pCRD.Name)
					c.UpwardController.AddToQueue(clusterName + "/" + pCRD.Name)
				}
			}
		}
	}
	metrics.CheckerMissMatchStats.WithLabelValues("MissMatchedCRD").Set(float64(numMissMatchedCRD))
}

func (c *controller) checkCRDOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterCrdController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing CRD from cluster %s informer cache: %v", clusterName, err)
		return
	}

	crdList := listObj.(*v1beta1.CustomResourceDefinitionList)
	vcrestconfig := c.multiClusterCrdController.GetCluster(clusterName).GetRestConfig()
	var vcapiextensionsClient apiextensionclientset.CustomResourceDefinitionsGetter

	if vcrestconfig == nil {
		klog.Errorf("cannot get virtual cluster restful config")
		return
	}
	vcc, err := apiextensionsclientset.NewForConfig(vcrestconfig)
	if err != nil {
		klog.Errorf("cannot create CRD client in virtual cluster ")
		return
	}
	vcapiextensionsClient = vcc.ApiextensionsV1beta1()

	for i, vCRD := range crdList.Items {
		if !publicCRD(&vCRD) {
			continue
		}
		pCRD := &v1beta1.CustomResourceDefinition{}
		err := c.superClient.Get(context.Background(), client.ObjectKey{
			Name: vCRD.Name,
		}, pCRD)
		if errors.IsNotFound(err) {
			opts := &metav1.DeleteOptions{
				PropagationPolicy: &constants.DefaultDeletionPolicy,
			}
			klog.Infof("patroller delete vcrd %v in virtual cluster %v", vCRD.Name, clusterName)
			err = vcapiextensionsClient.CustomResourceDefinitions().Delete(context.TODO(), vCRD.Name, *opts)
			if err != nil {
				klog.Errorf("error deleting CRD %v in cluster %s: %v", vCRD.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanTenantCRD").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("failed to get CRD  %s from super master cache: %v", vCRD.Name, err)
			continue
		}
		updatedCRD := conversion.Equality(nil, nil).CheckCRDEquality(pCRD, &crdList.Items[i])
		if updatedCRD != nil {
			atomic.AddUint64(&numMissMatchedCRD, 1)
			if publicCRD(pCRD) {
				klog.Infof("patroller update CRD %v in tenant cluster %v", vCRD.Name, clusterName)
				c.UpwardController.AddToQueue(clusterName + "/" + pCRD.Name)
			}
		}
	}
}
