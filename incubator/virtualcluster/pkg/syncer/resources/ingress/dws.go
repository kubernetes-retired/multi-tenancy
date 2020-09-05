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

	v1beta1 "k8s.io/api/extensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.ingressSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Ingress dws")
	}
	return c.multiClusterIngressController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile ingress %s/%s for cluster %s", request.Namespace, request.Name, request.ClusterName)
	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)
	pIngress, err := c.ingressLister.Ingresses(targetNamespace).Get(request.Name)
	pExists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		pExists = false
	}
	vExists := true
	vIngressObj, err := c.multiClusterIngressController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}

	if vExists && !pExists {
		vIngress := vIngressObj.(*v1beta1.Ingress)
		err := c.reconcileIngressCreate(request.ClusterName, targetNamespace, request.UID, vIngress)
		if err != nil {
			klog.Errorf("failed reconcile ingress %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		err := c.reconcileIngressRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pIngress)
		if err != nil {
			klog.Errorf("failed reconcile ingress %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if vExists && pExists {
		vIngress := vIngressObj.(*v1beta1.Ingress)
		err := c.reconcileIngressUpdate(request.ClusterName, targetNamespace, request.UID, pIngress, vIngress)
		if err != nil {
			klog.Errorf("failed reconcile ingress %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileIngressCreate(clusterName, targetNamespace, requestUID string, ingress *v1beta1.Ingress) error {
	vcName, _, _, err := c.multiClusterIngressController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcName, targetNamespace, ingress)
	if err != nil {
		return err
	}

	pIngress := newObj.(*v1beta1.Ingress)

	pIngress, err = c.ingressClient.Ingresses(targetNamespace).Create(context.TODO(), pIngress, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		if pIngress.Annotations[constants.LabelUID] == requestUID {
			klog.Infof("ingress %s/%s of cluster %s already exist in super master", targetNamespace, pIngress.Name, clusterName)
			return nil
		} else {
			return fmt.Errorf("pIngress %s/%s exists but its delegated object UID is different.", targetNamespace, pIngress.Name)
		}
	}
	return err
}

func (c *controller) reconcileIngressUpdate(clusterName, targetNamespace, requestUID string, pIngress, vIngress *v1beta1.Ingress) error {
	if pIngress.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("pIngress %s/%s delegated UID is different from updated object.", targetNamespace, pIngress.Name)
	}

	spec, err := c.multiClusterIngressController.GetSpec(clusterName)
	if err != nil {
		return err
	}
	updated := conversion.Equality(c.config, spec).CheckIngressEquality(pIngress, vIngress)
	if updated != nil {
		_, err = c.ingressClient.Ingresses(targetNamespace).Update(context.TODO(), updated, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) reconcileIngressRemove(clusterName, targetNamespace, requestUID, name string, pIngress *v1beta1.Ingress) error {
	if pIngress.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pIngress %s/%s delegated UID is different from deleted object.", targetNamespace, name)
	}

	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
		Preconditions:     metav1.NewUIDPreconditions(string(pIngress.UID)),
	}
	err := c.ingressClient.Ingresses(targetNamespace).Delete(context.TODO(), name, *opts)
	if errors.IsNotFound(err) {
		klog.Warningf("To be deleted ingress %s/%s not found in super master", targetNamespace, name)
		return nil
	}
	return err
}
