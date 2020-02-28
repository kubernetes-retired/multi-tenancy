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

package configmap

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
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
		err := c.reconcileConfigMapCreate(request.ClusterName, targetNamespace, vConfigMap)
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcileConfigMapRemove(request.ClusterName, targetNamespace, request.Name)
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vConfigMap := vConfigMapObj.(*v1.ConfigMap)
		err := c.reconcileConfigMapUpdate(request.ClusterName, targetNamespace, pConfigMap, vConfigMap)
		if err != nil {
			klog.Errorf("failed reconcile configmap %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileConfigMapCreate(clusterName, targetNamespace string, configMap *v1.ConfigMap) error {
	newObj, err := conversion.BuildMetadata(clusterName, targetNamespace, configMap)
	if err != nil {
		return err
	}

	_, err = c.configMapClient.ConfigMaps(targetNamespace).Create(newObj.(*v1.ConfigMap))
	if errors.IsAlreadyExists(err) {
		klog.Infof("configmap %s/%s of cluster %s already exist in super master", targetNamespace, configMap.Name, clusterName)
		return nil
	}
	return err
}

func (c *controller) reconcileConfigMapUpdate(clusterName, targetNamespace string, pConfigMap, vConfigMap *v1.ConfigMap) error {
	spec, err := c.multiClusterConfigMapController.GetSpec(clusterName)
	if err != nil {
		return err
	}
	updatedConfigMap := conversion.Equality(spec).CheckConfigMapEquality(pConfigMap, vConfigMap)
	if updatedConfigMap != nil {
		pConfigMap, err = c.configMapClient.ConfigMaps(targetNamespace).Update(updatedConfigMap)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) reconcileConfigMapRemove(clusterName, targetNamespace, name string) error {
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.configMapClient.ConfigMaps(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("configmap %s/%s of cluster %s not found in super master", targetNamespace, name, clusterName)
		return nil
	}
	return err
}
