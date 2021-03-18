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

package pod

import (
	"context"
	"fmt"
	"time"

	pkgerr "github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/metrics"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/featuregate"
	utilconstants "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	if !cache.WaitForCacheSync(stopCh, c.podSynced, c.serviceSynced, c.secretSynced) {
		return fmt.Errorf("failed to wait for caches to sync before starting Pod dws")
	}
	return c.MultiClusterController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (res reconciler.Result, retErr error) {
	klog.V(4).Infof("reconcile pod %s/%s for cluster %s", request.Namespace, request.Name, request.ClusterName)
	targetNamespace := conversion.ToSuperMasterNamespace(request.ClusterName, request.Namespace)

	pPod, err := c.podLister.Pods(targetNamespace).Get(request.Name)
	if err != nil && !errors.IsNotFound(err) {
		return reconciler.Result{Requeue: true}, err
	}

	var vPod *v1.Pod
	vPodObj, err := c.MultiClusterController.Get(request.ClusterName, request.Namespace, request.Name)
	if err == nil {
		vPod = vPodObj.(*v1.Pod)
	} else if !errors.IsNotFound(err) {
		return reconciler.Result{Requeue: true}, err
	}

	var operation string
	defer func() {
		recordOperationDuration(operation, time.Now())
		recordOperationStatus(operation, retErr)
	}()

	if vPod != nil && pPod == nil {
		operation = "pod_add"
		err := c.reconcilePodCreate(request.ClusterName, targetNamespace, request.UID, vPod)
		if err != nil {
			klog.Errorf("failed reconcile Pod %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)

			if parentRef := getParentRefFromPod(vPod); parentRef != nil {
				c.MultiClusterController.Eventf(request.ClusterName, parentRef, v1.EventTypeWarning, "FailedCreate", "Error creating: %v", err)
			}
			c.MultiClusterController.Eventf(request.ClusterName, &v1.ObjectReference{
				Kind:      "Pod",
				Name:      vPod.Name,
				Namespace: vPod.Namespace,
				UID:       vPod.UID,
			}, v1.EventTypeWarning, "FailedCreate", "Error creating: %v", err)

			return reconciler.Result{Requeue: true}, err
		}
	} else if vPod == nil && pPod != nil {
		operation = "pod_delete"
		err := c.reconcilePodRemove(request.ClusterName, targetNamespace, request.UID, request.Name, pPod)
		if err != nil {
			klog.Errorf("failed reconcile Pod %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.ClusterName, err)
			return reconciler.Result{Requeue: true}, err
		}
		if pPod.Spec.NodeName != "" {
			c.updateClusterVNodePodMap(request.ClusterName, pPod.Spec.NodeName, request.UID, reconciler.DeleteEvent)
		}
	} else if vPod != nil && pPod != nil {
		operation = "pod_update"
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
	_, cond := getPodCondition(&pod.Status, v1.PodScheduled)
	return cond != nil && cond.Status == v1.ConditionTrue
}

// getPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func getPodCondition(status *v1.PodStatus, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	return getPodConditionFromList(status.Conditions, conditionType)
}

// getPodConditionFromList extracts the provided condition from the given list of condition and
// returns the index of the condition and the condition. Returns -1 and nil if the condition is not present.
func getPodConditionFromList(conditions []v1.PodCondition, conditionType v1.PodConditionType) (int, *v1.PodCondition) {
	if conditions == nil {
		return -1, nil
	}
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return i, &conditions[i]
		}
	}
	return -1, nil
}

func getParentRefFromPod(vPod *v1.Pod) *v1.ObjectReference {
	if len(vPod.OwnerReferences) == 0 {
		return nil
	}

	owner := vPod.OwnerReferences[0]
	return &v1.ObjectReference{
		Kind:      owner.Kind,
		Namespace: vPod.Namespace,
		Name:      owner.Name,
		UID:       owner.UID,
	}
}

func (c *controller) reconcilePodCreate(clusterName, targetNamespace, requestUID string, vPod *v1.Pod) error {
	// load deleting pod, don't create any pod on super master.
	if vPod.DeletionTimestamp != nil {
		return nil
	}

	if vPod.Spec.NodeName != "" {
		// For now, we skip vPod that has NodeName set to prevent tenant from deploying DaemonSet or DaemonSet alike CRDs.
		err := c.MultiClusterController.Eventf(clusterName, &v1.ObjectReference{
			Kind:      "Pod",
			Name:      vPod.Name,
			Namespace: vPod.Namespace,
			UID:       vPod.UID,
		}, v1.EventTypeWarning, "NotSupported", "The Pod has nodeName set in the spec which is not supported for now")
		return err
	}

	vcName, vcNS, _, err := c.MultiClusterController.GetOwnerInfo(clusterName)
	if err != nil {
		return err
	}
	newObj, err := conversion.BuildMetadata(clusterName, vcNS, vcName, targetNamespace, vPod)
	if err != nil {
		return err
	}

	pPod := newObj.(*v1.Pod)

	pSecretMap, err := c.findPodServiceAccountSecret(clusterName, pPod, vPod)
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
		conversion.PodMutateServiceLink(c.Config.DisablePodServiceLinks),
		conversion.PodMutateDefault(vPod, pSecretMap, services, nameServer),
		conversion.PodMutateAutoMountServiceAccountToken(c.Config.DisableServiceAccountToken),
		// TODO: make extension configurable
		//conversion.PodAddExtensionMeta(vPod),
	}

	err = conversion.VC(c.MultiClusterController, clusterName).Pod(pPod).Mutate(ms...)
	if err != nil {
		return fmt.Errorf("failed to mutate pod: %v", err)
	}
	pPod, err = c.client.Pods(targetNamespace).Create(context.TODO(), pPod, metav1.CreateOptions{})
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

func (c *controller) findPodServiceAccountSecret(clusterName string, pPod, vPod *v1.Pod) (map[string]string, error) {
	mountSecretSet := sets.NewString()
	for _, volume := range vPod.Spec.Volumes {
		if volume.Secret != nil {
			mountSecretSet.Insert(volume.Secret.SecretName)
		}
	}

	// vSecretName -> pSecretName
	mutateNameMap := make(map[string]string)

	for secretName := range mountSecretSet {
		vSecretObj, err := c.MultiClusterController.GetByObjectType(clusterName, vPod.Namespace, secretName, &v1.Secret{})
		if err != nil {
			return nil, pkgerr.Wrapf(err, "failed to get vSecret %s/%s", vPod.Namespace, secretName)
		}
		vSecret := vSecretObj.(*v1.Secret)

		// normal secret. pSecret name is the same as the vSecret.
		if vSecret.Type != v1.SecretTypeServiceAccountToken {
			continue
		}

		secretList, err := c.secretLister.Secrets(pPod.Namespace).List(labels.SelectorFromSet(map[string]string{
			constants.LabelSecretUID: string(vSecret.UID),
		}))
		if err != nil || len(secretList) == 0 {
			return nil, fmt.Errorf("failed to find sa secret from super master %s/%s: %v", pPod.Namespace, vSecret.UID, err)
		}

		mutateNameMap[secretName] = secretList[0].Name
	}

	return mutateNameMap, nil
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
	if featuregate.DefaultFeatureGate.Enabled(featuregate.SuperClusterServiceNetwork) {
		apiserver, err := c.serviceLister.Services(cluster).Get("apiserver-svc")
		if err != nil {
			return nil, err
		}
		services = append(services, apiserver)
	}

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
		err := c.client.Pods(targetNamespace).Delete(context.TODO(), pPod.Name, *deleteOptions)
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	vc, err := util.GetVirtualClusterObject(c.MultiClusterController, clusterName)
	if err != nil {
		return err
	}
	updatedPod := conversion.Equality(c.Config, vc).CheckPodEquality(pPod, vPod)
	if updatedPod != nil {
		pPod, err = c.client.Pods(targetNamespace).Update(context.TODO(), updatedPod, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	}
	updatedPodStatus := conversion.CheckDWPodConditionEquality(pPod, vPod)
	if updatedPodStatus != nil {
		updatedPod = pPod.DeepCopy()
		updatedPod.Status = *updatedPodStatus
		pPod, err = c.client.Pods(targetNamespace).UpdateStatus(context.TODO(), updatedPod, metav1.UpdateOptions{})
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
	err := c.client.Pods(targetNamespace).Delete(context.TODO(), name, *opts)
	if errors.IsNotFound(err) {
		klog.Warningf("To be deleted pod %s/%s of cluster (%s) is not found in super master", targetNamespace, name, clusterName)
		return nil
	}
	return err
}

func recordOperationDuration(operation string, start time.Time) {
	metrics.PodOperationsDuration.WithLabelValues(operation).Observe(metrics.SinceInSeconds(start))
}

func recordOperationStatus(operation string, err error) {
	if err != nil {
		metrics.PodOperations.With(prometheus.Labels{"operation_type": operation, "code": utilconstants.StatusCodeError}).Inc()
		return
	}
	metrics.PodOperations.With(prometheus.Labels{"operation_type": operation, "code": utilconstants.StatusCodeOK}).Inc()
}
