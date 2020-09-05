/*
Copyright 2020 The Kubernetes Authors.

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

package ingress

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
)

var numSpecMissMatchedIngresses uint64
var numStatusMissMatchedIngresses uint64
var numUWMetaMissMatchedIngresses uint64

func (c *controller) StartPatrol(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.ingressSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Ingress checker")
	}
	c.ingressPatroller.Start(stopCh)
	return nil
}

// PatrollerDo check if ingresss keep consistency between super
// master and tenant masters.
func (c *controller) PatrollerDo() {
	clusterNames := c.multiClusterIngressController.GetClusterNames()
	if len(clusterNames) == 0 {
		klog.Infof("tenant masters has no clusters, give up period checker")
		return
	}

	wg := sync.WaitGroup{}
	numSpecMissMatchedIngresses = 0
	numStatusMissMatchedIngresses = 0
	numUWMetaMissMatchedIngresses = 0

	for _, clusterName := range clusterNames {
		wg.Add(1)
		go func(clusterName string) {
			defer wg.Done()
			c.checkIngressesOfTenantCluster(clusterName)
		}(clusterName)
	}
	wg.Wait()

	pIngresses, err := c.ingressLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("error listing ingresss from super master informer cache: %v", err)
		return
	}

	for _, pIngress := range pIngresses {
		clusterName, vNamespace := conversion.GetVirtualOwner(pIngress)
		if len(clusterName) == 0 || len(vNamespace) == 0 {
			continue
		}
		shouldDelete := false
		vIngressObj, err := c.multiClusterIngressController.Get(clusterName, vNamespace, pIngress.Name)
		if errors.IsNotFound(err) {
			shouldDelete = true
		}
		if err == nil {
			vIngress := vIngressObj.(*v1beta1.Ingress)
			if pIngress.Annotations[constants.LabelUID] != string(vIngress.UID) {
				shouldDelete = true
				klog.Warningf("Found pIngress %s/%s delegated UID is different from tenant object.", pIngress.Namespace, pIngress.Name)
			}
		}
		if shouldDelete {
			deleteOptions := metav1.NewPreconditionDeleteOptions(string(pIngress.UID))
			if err = c.ingressClient.Ingresses(pIngress.Namespace).Delete(context.TODO(), pIngress.Name, *deleteOptions); err != nil {
				klog.Errorf("error deleting pIngress %s/%s in super master: %v", pIngress.Namespace, pIngress.Name, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("DeletedOrphanSuperMasterIngresses").Inc()
			}
		}
	}

	metrics.CheckerMissMatchStats.WithLabelValues("SpecMissMatchedIngresses").Set(float64(numSpecMissMatchedIngresses))
	metrics.CheckerMissMatchStats.WithLabelValues("StatusMissMatchedIngresses").Set(float64(numStatusMissMatchedIngresses))
	metrics.CheckerMissMatchStats.WithLabelValues("UWMetaMissMatchedIngresses").Set(float64(numUWMetaMissMatchedIngresses))
}

func (c *controller) checkIngressesOfTenantCluster(clusterName string) {
	listObj, err := c.multiClusterIngressController.List(clusterName)
	if err != nil {
		klog.Errorf("error listing ingresss from cluster %s informer cache: %v", clusterName, err)
		return
	}
	klog.V(4).Infof("check ingresss consistency in cluster %s", clusterName)
	ingList := listObj.(*v1beta1.IngressList)
	for i, vIngress := range ingList.Items {
		targetNamespace := conversion.ToSuperMasterNamespace(clusterName, vIngress.Namespace)
		pIngress, err := c.ingressLister.Ingresses(targetNamespace).Get(vIngress.Name)
		if errors.IsNotFound(err) {
			if err := c.multiClusterIngressController.RequeueObject(clusterName, &ingList.Items[i]); err != nil {
				klog.Errorf("error requeue vingress %v/%v in cluster %s: %v", vIngress.Namespace, vIngress.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantIngresses").Inc()
			}
			continue
		}

		if err != nil {
			klog.Errorf("failed to get pIngress %s/%s from super master cache: %v", targetNamespace, vIngress.Name, err)
			continue
		}

		if pIngress.Annotations[constants.LabelUID] != string(vIngress.UID) {
			klog.Errorf("Found pIngress %s/%s delegated UID is different from tenant object.", targetNamespace, pIngress.Name)
			continue
		}

		spec, err := c.multiClusterIngressController.GetSpec(clusterName)
		if err != nil {
			klog.Errorf("fail to get cluster spec : %s", clusterName)
			continue
		}
		updatedIngress := conversion.Equality(c.config, spec).CheckIngressEquality(pIngress, &ingList.Items[i])
		if updatedIngress != nil {
			atomic.AddUint64(&numSpecMissMatchedIngresses, 1)
			klog.Warningf("spec of ingress %v/%v diff in super&tenant master", vIngress.Namespace, vIngress.Name)
			if err := c.multiClusterIngressController.RequeueObject(clusterName, &ingList.Items[i]); err != nil {
				klog.Errorf("error requeue vingress %v/%v in cluster %s: %v", vIngress.Namespace, vIngress.Name, clusterName, err)
			} else {
				metrics.CheckerRemedyStats.WithLabelValues("RequeuedTenantIngresses").Inc()
			}
		}

		enqueue := false
		updatedMeta := conversion.Equality(c.config, spec).CheckUWObjectMetaEquality(&pIngress.ObjectMeta, &ingList.Items[i].ObjectMeta)
		if updatedMeta != nil {
			atomic.AddUint64(&numUWMetaMissMatchedIngresses, 1)
			enqueue = true
			klog.Warningf("UWObjectMeta of vIngress %v/%v diff in super&tenant master", vIngress.Namespace, vIngress.Name)
		}
		if !equality.Semantic.DeepEqual(vIngress.Status, pIngress.Status) {
			enqueue = true
			atomic.AddUint64(&numStatusMissMatchedIngresses, 1)
			klog.Warningf("Status of vIngress %v/%v diff in super&tenant master", vIngress.Namespace, vIngress.Name)
		}
		if enqueue {
			c.enqueueIngress(pIngress)
		}

	}
}
