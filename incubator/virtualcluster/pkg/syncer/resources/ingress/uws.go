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

package ingress

import (
	"context"
	"fmt"

	pkgerr "github.com/pkg/errors"
	v1beta1 "k8s.io/api/extensions/v1beta1"
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
	if !cache.WaitForCacheSync(stopCh, c.ingressSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.upwardIngressController.Start(stopCh)
}

func (c *controller) BackPopulate(key string) error {
	pNamespace, pName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key %v: %v", key, err))
		return nil
	}

	pIngress, err := c.ingressLister.Ingresses(pNamespace).Get(pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	clusterName, vNamespace := conversion.GetVirtualOwner(pIngress)
	if clusterName == "" || vNamespace == "" {
		klog.Infof("drop ingress %s/%s which is not belongs to any tenant", pNamespace, pName)
		return nil
	}

	vIngressObj, err := c.multiClusterIngressController.Get(clusterName, vNamespace, pName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return pkgerr.Wrapf(err, "could not find pIngress %s/%s's vIngress in controller cache", vNamespace, pName)
	}
	vIngress := vIngressObj.(*v1beta1.Ingress)
	if pIngress.Annotations[constants.LabelUID] != string(vIngress.UID) {
		return fmt.Errorf("BackPopulated pIngress %s/%s delegated UID is different from updated object.", pIngress.Namespace, pIngress.Name)
	}

	tenantClient, err := c.multiClusterIngressController.GetClusterClient(clusterName)
	if err != nil {
		return pkgerr.Wrapf(err, "failed to create client from cluster %s config", clusterName)
	}

	spec, err := util.GetVirtualClusterSpec(c.multiClusterIngressController, clusterName)
	if err != nil {
		return pkgerr.Wrapf(err, "failed to get spec of cluster %s", clusterName)
	}

	var newIngress *v1beta1.Ingress
	updatedMeta := conversion.Equality(c.config, spec).CheckUWObjectMetaEquality(&pIngress.ObjectMeta, &vIngress.ObjectMeta)
	if updatedMeta != nil {
		newIngress = vIngress.DeepCopy()
		newIngress.ObjectMeta = *updatedMeta
		if _, err = tenantClient.ExtensionsV1beta1().Ingresses(vIngress.Namespace).Update(context.TODO(), newIngress, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to back populate ingress %s/%s meta update for cluster %s: %v", vIngress.Namespace, vIngress.Name, clusterName, err)
		}
	}

	if !equality.Semantic.DeepEqual(vIngress.Status, pIngress.Status) {
		if newIngress == nil {
			newIngress = vIngress.DeepCopy()
		} else {
			// vIngress has been updated, let us fetch the lastest version.
			if newIngress, err = tenantClient.ExtensionsV1beta1().Ingresses(vIngress.Namespace).Get(context.TODO(), vIngress.Name, metav1.GetOptions{}); err != nil {
				return fmt.Errorf("failed to retrieve vIngress %s/%s from cluster %s: %v", vIngress.Namespace, vIngress.Name, clusterName, err)
			}
		}
		newIngress.Status = pIngress.Status
		if _, err = tenantClient.ExtensionsV1beta1().Ingresses(vIngress.Namespace).UpdateStatus(context.TODO(), newIngress, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to back populate ingress %s/%s status update for cluster %s: %v", vIngress.Namespace, vIngress.Name, clusterName, err)
		}
	}
	return nil
}
