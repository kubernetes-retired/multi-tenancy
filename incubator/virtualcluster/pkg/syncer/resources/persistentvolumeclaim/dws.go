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

package persistentvolumeclaim

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
	if !cache.WaitForCacheSync(stopCh, c.pvcSynced) {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	return c.multiClusterPersistentVolumeClaimController.Start(stopCh)
}

// The reconcile logic for tenant master pvc informer
func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile pvc %s/%s event for cluster %s", request.Namespace, request.Name, request.ClusterName)

	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)
	pPVC, err := c.pvcLister.PersistentVolumeClaims(targetNamespace).Get(request.Name)
	pExists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		pExists = false
	}
	vExists := true
	vPVCObj, err := c.multiClusterPersistentVolumeClaimController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}

	if vExists && !pExists {
		vPVC := vPVCObj.(*v1.PersistentVolumeClaim)
		err := c.reconcilePVCCreate(request.ClusterName, targetNamespace, request.UID, vPVC)
		if err != nil {
			klog.Errorf("failed reconcile pvc %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcilePVCRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pPVC)
		if err != nil {
			klog.Errorf("failed reconcile pvc %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vPVC := vPVCObj.(*v1.PersistentVolumeClaim)
		err := c.reconcilePVCUpdate(request.ClusterName, targetNamespace, request.UID, pPVC, vPVC)
		if err != nil {
			klog.Errorf("failed reconcile pvc %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcilePVCCreate(clusterName, targetNamespace, requestUID string, pvc *v1.PersistentVolumeClaim) error {
	vcName, vcNS, _, err := c.multiClusterPersistentVolumeClaimController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcNS, vcName, targetNamespace, pvc)
	if err != nil {
		return err
	}

	pPVC := newObj.(*v1.PersistentVolumeClaim)

	pPVC, err = c.pvcClient.PersistentVolumeClaims(targetNamespace).Create(context.TODO(), pPVC, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		if pPVC.Annotations[constants.LabelUID] == requestUID {
			klog.Infof("pvc %s/%s of cluster %s already exist in super master", targetNamespace, pPVC.Name, clusterName)
			return nil
		} else {
			return fmt.Errorf("pPVC %s/%s exists but its delegated object UID is different.", targetNamespace, pPVC.Name)
		}
	}
	return err
}

func (c *controller) reconcilePVCUpdate(clusterName, targetNamespace, requestUID string, pPVC, vPVC *v1.PersistentVolumeClaim) error {
	if pPVC.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("pPVC %s/%s delegated UID is different from updated object.", targetNamespace, pPVC.Name)
	}
	vc, err := util.GetVirtualClusterObject(c.multiClusterPersistentVolumeClaimController, clusterName)
	if err != nil {
		return err
	}
	updatedPVC := conversion.Equality(c.config, vc).CheckPVCEquality(pPVC, vPVC)
	if updatedPVC != nil {
		pPVC, err = c.pvcClient.PersistentVolumeClaims(targetNamespace).Update(context.TODO(), updatedPVC, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) reconcilePVCRemove(clusterName, targetNamespace, requestUID, name string, pPVC *v1.PersistentVolumeClaim) error {
	if pPVC.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pPVC %s/%s delegated UID is different from deleted object.", targetNamespace, pPVC.Name)
	}
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.pvcClient.PersistentVolumeClaims(targetNamespace).Delete(context.TODO(), name, *opts)
	if errors.IsNotFound(err) {
		klog.Warningf("pvc %s/%s of cluster %s not found in super master", targetNamespace, name, clusterName)
		return nil
	}
	return err
}
