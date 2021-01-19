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

package configmap

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.configMapSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.multiClusterConfigMapController.Start(stopCh)
}

// The reconcile logic for tenant master configMap informer
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile configmap %s/%s event for cluster %s", request.Namespace, request.Name, request.ClusterName)

	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)
	pConfigMap, err := c.configMapLister.ConfigMaps(targetNamespace).Get(request.Name)
	pExists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		pExists = false
	}
	vExists := true
	vConfigMapObj, err := c.multiClusterConfigMapController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}

	if vExists && !pExists {
		vConfigMap := vConfigMapObj.(*v1.ConfigMap)
		err := c.reconcileConfigMapCreate(request.ClusterName, targetNamespace, request.UID, vConfigMap)
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcileConfigMapRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pConfigMap)
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vConfigMap := vConfigMapObj.(*v1.ConfigMap)
		err := c.reconcileConfigMapUpdate(request.ClusterName, targetNamespace, request.UID, pConfigMap, vConfigMap)
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileConfigMapCreate(clusterName, targetNamespace, requestUID string, configMap *v1.ConfigMap) error {
	vcName, vcNS, _, err := c.multiClusterConfigMapController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcNS, vcName, targetNamespace, configMap)
	if err != nil {
		return err
	}

	pConfigMap, err := c.configMapClient.ConfigMaps(targetNamespace).Create(context.TODO(), newObj.(*v1.ConfigMap), metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		if pConfigMap.Annotations[constants.LabelUID] == requestUID {
			klog.Infof("configmap %s/%s of cluster %s already exist in super master", targetNamespace, configMap.Name, clusterName)
			return nil
		} else {
			return fmt.Errorf("pConfigMap %s/%s exists but its delegated object UID is different.", targetNamespace, pConfigMap.Name)
		}
	}
	return err
}

func (c *controller) reconcileConfigMapUpdate(clusterName, targetNamespace, requestUID string, pConfigMap, vConfigMap *v1.ConfigMap) error {
	if pConfigMap.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("pConfigMap %s/%s delegated UID is different from updated object.", targetNamespace, pConfigMap.Name)
	}
	spec, err := util.GetVirtualClusterSpec(c.multiClusterConfigMapController, clusterName)
	if err != nil {
		return err
	}
	updatedConfigMap := conversion.Equality(c.config, spec).CheckConfigMapEquality(pConfigMap, vConfigMap)
	if updatedConfigMap != nil {
		pConfigMap, err = c.configMapClient.ConfigMaps(targetNamespace).Update(context.TODO(), updatedConfigMap, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) reconcileConfigMapRemove(clusterName, targetNamespace, requestUID, name string, pConfigMap *v1.ConfigMap) error {
	if pConfigMap.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pConfigMap %s/%s delegated UID is different from deleted object.", targetNamespace, name)
	}
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.configMapClient.ConfigMaps(targetNamespace).Delete(context.TODO(), name, *opts)
	if errors.IsNotFound(err) {
		klog.Warningf("configmap %s/%s of cluster %s not found in super master", targetNamespace, name, clusterName)
		return nil
	}
	return err
}
