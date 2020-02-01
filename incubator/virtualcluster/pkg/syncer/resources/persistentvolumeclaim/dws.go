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

package persistentvolumeclaim

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
	if !cache.WaitForCacheSync(stopCh, c.pvcSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.multiClusterPersistentVolumeClaimController.Start(stopCh)
}

// The reconcile logic for tenant master pvc informer
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile pvc %s %s event for cluster %s", request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcilePVCCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.PersistentVolumeClaim))
		if err != nil {
			klog.Errorf("failed reconcile pvc  %s CREATE of cluster %s %v", request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, nil
		}
	case reconciler.UpdateEvent:
		err := c.reconcilePVCUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.PersistentVolumeClaim))
		if err != nil {
			klog.Errorf("failed reconcile pvc %s UPDATE of cluster %s %v", request.Name, request.Cluster.Name, err)
			return reconciler.Result{}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcilePVCRemove(request.Cluster.Name, request.Namespace, request.Name)
		if err != nil {
			klog.Errorf("failed reconcile pvc %s DELETE of cluster %s %v", request.Name, request.Cluster.Name, err)
			return reconciler.Result{}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcilePVCCreate(cluster, namespace, name string, pvc *v1.PersistentVolumeClaim) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	_, err := c.pvcLister.PersistentVolumeClaims(targetNamespace).Get(name)
	if err == nil {
		return c.reconcilePVCUpdate(cluster, namespace, name, pvc)
	}

	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, pvc)
	if err != nil {
		return err
	}

	pPVC := newObj.(*v1.PersistentVolumeClaim)

	_, err = c.pvcClient.PersistentVolumeClaims(targetNamespace).Create(pPVC)
	if errors.IsAlreadyExists(err) {
		klog.Infof("pvc %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcilePVCUpdate(cluster, namespace, name string, vPVC *v1.PersistentVolumeClaim) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	pPVC, err := c.pvcLister.PersistentVolumeClaims(targetNamespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	updatedPVC := conversion.CheckPVCEquality(pPVC, vPVC)
	if updatedPVC != nil {
		pPVC, err = c.pvcClient.PersistentVolumeClaims(targetNamespace).Update(updatedPVC)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) reconcilePVCRemove(cluster, namespace, name string) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.pvcClient.PersistentVolumeClaims(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("pvc %s/%s of cluster not found in super master", namespace, name)
		return nil
	}
	return err
}
