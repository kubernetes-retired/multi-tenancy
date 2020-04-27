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

package pod

import (
	"fmt"
	"time"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.podSynced, c.serviceSynced, c.secretSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Pod dws")
	}
	return c.multiClusterPodController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.V(4).Infof("reconcile pod %s/%s for cluster %s", request.Namespace, request.Name, request.ClusterName)
	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)
	pPod, err := c.podLister.Pods(targetNamespace).Get(request.Name)
	pExists := true
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		pExists = false
	}
	vExists := true
	vPodObj, err := c.multiClusterPodController.Get(request.ClusterName, request.Namespace, request.Name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return reconciler.Result{Requeue: true}, err
		}
		vExists = false
	}

	var operation string
	if vExists && !pExists {
		operation = "pod_add"
		defer recordOperation(operation, time.Now())
		vPod := vPodObj.(*v1.Pod)
		err := c.reconcilePodCreate(request.ClusterName, targetNamespace, request.UID, vPod)
		if err != nil {
			klog.Errorf("failed reconcile Pod %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
	} else if !vExists && pExists {
		operation = "pod_delete"
		defer recordOperation(operation, time.Now())
		err := c.reconcilePodRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pPod)
		if err != nil {
			klog.Errorf("failed reconcile Pod %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
		if pPod.Spec.NodeName != "" {
			c.updateClusterVNodePodMap(request.ClusterName, pPod.Spec.NodeName, request.UID, reconciler.DeleteEvent)
		}
	} else if vExists && pExists {
		operation = "pod_update"
		defer recordOperation(operation, time.Now())
		vPod := vPodObj.(*v1.Pod)
		err := c.reconcilePodUpdate(request.ClusterName, targetNamespace, request.UID, pPod, vPod)
		if err != nil {
			klog.Errorf("failed reconcile Pod %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
		if vPod.Spec.NodeName != "" {
			c.updateClusterVNodePodMap(request.ClusterName, vPod.Spec.NodeName, request.UID, reconciler.UpdateEvent)
		}
	} else {
		// object is gone.
	}
	return reconciler.Result{}, nil
}

func isPodScheduled(pod *v1.Pod) bool {
	_, cond := podutil.GetPodCondition(&pod.Status, v1.PodScheduled)
	return cond != nil && cond.Status == v1.ConditionTrue
}

func createNotSupportEvent(pod *v1.Pod) *v1.Event {
	eventTime := metav1.Now()
	return &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "syncer",
		},
		InvolvedObject: v1.ObjectReference{
			APIVersion: "v1",
			Kind:       "Pod",
			Name:       pod.Name,
		},
		Type:                "Warning",
		Reason:              "NotSupported",
		Message:             "The Pod has nodeName set in the spec which is not supported for now",
		FirstTimestamp:      eventTime,
		LastTimestamp:       eventTime,
		ReportingController: "syncer",
	}
}

func (c *controller) reconcilePodCreate(clusterName, targetNamespace, requestUID string, vPod *v1.Pod) error {
	// load deleting pod, don't create any pod on super master.
	if vPod.DeletionTimestamp != nil {
		return nil
	}

	if vPod.Spec.NodeName != "" {
		// For now, we skip vPod that has NodeName set to prevent tenant from deploying DaemonSet or DaemonSet alike CRDs.
		tenantClient, err := c.multiClusterPodController.GetClusterClient(clusterName)
		if err != nil {
			return fmt.Errorf("failed to create client from cluster %s config: %v", clusterName, err)
		}
		event := createNotSupportEvent(vPod)
		vEvent := conversion.BuildVirtualPodEvent(clusterName, event, vPod)
		_, err = tenantClient.CoreV1().Events(vPod.Namespace).Create(vEvent)
		return err
	}

	newObj, err := conversion.BuildMetadata(clusterName, targetNamespace, vPod)
	if err != nil {
		return err
	}

	pPod := newObj.(*v1.Pod)

	pSecret, err := c.getPodServiceAccountSecret(clusterName, pPod, vPod)
	if err != nil {
		return fmt.Errorf("failed to get service account secret from cluster %s cache: %v", clusterName, err)
	}

	services, err := c.getPodRelatedServices(clusterName, pPod)
	if err != nil {
		return fmt.Errorf("failed to list services from cluster %s cache: %v", clusterName, err)
	}

	nameServer, err := c.getClusterNameServer(clusterName)
	if err != nil {
		return fmt.Errorf("failed to find nameserver: %v", err)
	}

	var ms = []conversion.PodMutator{
		conversion.PodMutateDefault(vPod, pSecret, services, nameServer),
		conversion.PodMutateAutoMountServiceAccountToken(c.config.DisableServiceAccountToken),
		// TODO: make extension configurable
		//conversion.PodAddExtensionMeta(vPod),
	}

	err = conversion.VC(c.multiClusterPodController, clusterName).Pod(pPod).Mutate(ms...)
	if err != nil {
		return fmt.Errorf("failed to mutate pod: %v", err)
	}
	pPod, err = c.client.Pods(targetNamespace).Create(pPod)
	if errors.IsAlreadyExists(err) {
		if pPod.Annotations[constants.LabelUID] == requestUID {
			klog.Infof("pod %s/%s of cluster %s already exist in super master", targetNamespace, pPod.Name, clusterName)
			return nil
		} else {
			return fmt.Errorf("pPod %s/%s exists but the UID is different from tenant master.", targetNamespace, pPod.Name)
		}
	}

	return err
}

func (c *controller) getPodServiceAccountSecret(clusterName string, pPod, vPod *v1.Pod) (*v1.Secret, error) {
	saName := "default"
	if pPod.Spec.ServiceAccountName != "" {
		saName = pPod.Spec.ServiceAccountName
	}
	// find tenant sa UID
	vSaObj, err := c.multiClusterPodController.GetByObjectType(clusterName, vPod.Namespace, saName, &v1.ServiceAccount{})
	if err != nil {
		return nil, fmt.Errorf("fail to get tenant service account UID")
	}
	vSa := vSaObj.(*v1.ServiceAccount)

	// find service account token secret and replace the one set by tenant kcm.
	secretList, err := c.secretLister.Secrets(pPod.Namespace).List(labels.SelectorFromSet(map[string]string{
		constants.LabelServiceAccountUID: string(vSa.UID),
	}))
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	if secretList == nil || len(secretList) == 0 {
		return nil, fmt.Errorf("service account token secret for pod is not ready")
	}
	return secretList[0], nil
}

func (c *controller) getClusterNameServer(cluster string) (string, error) {
	svc, err := c.serviceLister.Services(conversion.ToSuperMasterNamespace(cluster, constants.TenantDNSServerNS)).Get(constants.TenantDNSServerServiceName)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return svc.Spec.ClusterIP, nil
}

func (c *controller) getPodRelatedServices(cluster string, pPod *v1.Pod) ([]*v1.Service, error) {
	var services []*v1.Service
	list, err := c.serviceLister.Services(conversion.ToSuperMasterNamespace(cluster, metav1.NamespaceDefault)).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	services = append(services, list...)

	list, err = c.serviceLister.Services(pPod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	services = append(services, list...)
	if len(services) == 0 {
		return nil, fmt.Errorf("service is not ready")
	}
	return services, nil
}

func (c *controller) reconcilePodUpdate(clusterName, targetNamespace, requestUID string, pPod, vPod *v1.Pod) error {
	if pPod.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("pPod %s/%s delegated UID is different from updated object.", targetNamespace, pPod.Name)
	}

	if vPod.DeletionTimestamp != nil {
		if pPod.DeletionTimestamp != nil {
			// pPod is under deletion, waiting for UWS bock populate the pod status.
			return nil
		}
		deleteOptions := metav1.NewDeleteOptions(*vPod.DeletionGracePeriodSeconds)
		deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pPod.UID))
		err := c.client.Pods(targetNamespace).Delete(pPod.Name, deleteOptions)
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	spec, err := c.multiClusterPodController.GetSpec(clusterName)
	if err != nil {
		return err
	}
	updatedPod := conversion.Equality(c.config, spec).CheckPodEquality(pPod, vPod)
	if updatedPod != nil {
		pPod, err = c.client.Pods(targetNamespace).Update(updatedPod)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *controller) reconcilePodRemove(clusterName, targetNamespace, requestUID, name string, pPod *v1.Pod) error {
	if pPod.Annotations[constants.LabelUID] != requestUID {
		return fmt.Errorf("To be deleted pPod %s/%s delegated UID is different from deleted object.", targetNamespace, name)
	}

	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
		Preconditions:     metav1.NewUIDPreconditions(string(pPod.UID)),
	}
	err := c.client.Pods(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("To be deleted pod %s/%s of cluster (%s) is not found in super master", targetNamespace, name, clusterName)
		return nil
	}
	return err
}

func recordOperation(operation string, start time.Time) {
	metrics.PodOperations.WithLabelValues(operation).Inc()
	metrics.PodOperationsDuration.WithLabelValues(operation).Observe(metrics.SinceInSeconds(start))
}

func recordError(operation string, err error) {
	if err != nil {
		metrics.PodOperationsErrors.WithLabelValues(operation).Inc()
	}
}
