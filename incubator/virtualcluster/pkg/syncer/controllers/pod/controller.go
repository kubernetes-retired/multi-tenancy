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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	ctrl "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/utils"
)

type controller struct {
	client                    v1core.CoreV1Interface
	multiClusterPodController *sc.MultiClusterController
	informer                  coreinformers.Interface

	workers       int
	podLister     listersv1.PodLister
	queue         workqueue.RateLimitingInterface
	podSynced     cache.InformerSynced
	serviceSynced cache.InformerSynced
}

func Register(
	client v1core.CoreV1Interface,
	informer coreinformers.Interface,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		client:   client,
		informer: informer,
		queue:    workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "super_master_pod"),
		workers:  constants.DefaultControllerWorkers,
	}

	// Create the multi cluster pod controller
	options := sc.Options{Reconciler: c}
	multiClusterPodController, err := sc.NewController("tenant-masters-pod-controller", &v1.Pod{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster pod controller %v", err)
		return
	}
	c.multiClusterPodController = multiClusterPodController

	c.podLister = informer.Pods().Lister()
	c.podSynced = informer.Pods().Informer().HasSynced
	c.serviceSynced = informer.Services().Informer().HasSynced
	informer.Pods().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
				case *v1.Pod:
					return assignedPod(t)
				case cache.DeletedFinalStateUnknown:
					if pod, ok := t.Obj.(*v1.Pod); ok {
						return assignedPod(pod)
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %T to *v1.Pod in %T", obj, c))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in %T: %T", c, obj))
					return false
				}
			},
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc: c.enqueuePod,
				UpdateFunc: func(oldObj, newObj interface{}) {
					newPod := newObj.(*v1.Pod)
					oldPod := oldObj.(*v1.Pod)
					if newPod.ResourceVersion == oldPod.ResourceVersion {
						// Periodic resync will send update events for all known Deployments.
						// Two different versions of the same Deployment will always have different RVs.
						return
					}

					c.enqueuePod(newObj)
				},
				DeleteFunc: c.enqueuePod,
			},
		},
	)

	controllerManager.AddController(c)
}

func (c *controller) enqueuePod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		return
	}

	clusterName, vNamespace := conversion.GetOwner(pod)
	if clusterName == "" {
		return
	}

	c.queue.Add(podQueueKey{
		clusterName: clusterName,
		vNamespace:  vNamespace,
		namespace:   pod.Namespace,
		name:        pod.Name,
	})
}

func (c *controller) StartDWS(stopCh <-chan struct{}) error {
	return c.multiClusterPodController.Start(stopCh)
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile pod %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcilePodCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Pod))
		if err != nil {
			klog.Errorf("failed reconcile pod %s/%s CREATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcilePodUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Pod))
		if err != nil {
			klog.Errorf("failed reconcile pod %s/%s UPDATE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcilePodRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Pod))
		if err != nil {
			klog.Errorf("failed reconcile pod %s/%s DELETE of cluster %s %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcilePodCreate(cluster, namespace, name string, vPod *v1.Pod) error {
	// load deleting pod, don't create any pod on super master.
	if vPod.DeletionTimestamp != nil {
		return c.reconcilePodUpdate(cluster, namespace, name, vPod)
	}

	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	_, err := c.podLister.Pods(targetNamespace).Get(name)
	if err == nil {
		return c.reconcilePodUpdate(cluster, namespace, name, vPod)
	}

	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, vPod)
	if err != nil {
		return err
	}

	pPod := newObj.(*v1.Pod)

	// check if the secret in super master is ready
	// we must create pod after sync the secret.
	saName := "default"
	if pPod.Spec.ServiceAccountName != "" {
		saName = pPod.Spec.ServiceAccountName
	}

	pSecret, err := utils.GetSecret(c.client, targetNamespace, saName)
	if err != nil {
		return fmt.Errorf("failed to get secret: %v", err)
	}

	if pSecret.Labels[constants.SyncStatusKey] != constants.SyncStatusReady {
		return fmt.Errorf("secret for pod is not ready")
	}

	var client *clientset.Clientset
	innerCluster := c.multiClusterPodController.GetCluster(cluster)
	if innerCluster == nil {
		klog.Infof("cluster %s is gone", cluster)
		return nil
	}
	client, err = innerCluster.GetClient()
	if err != nil {
		return err
	}
	vSecret, err := utils.GetSecret(client.CoreV1(), namespace, saName)
	if err != nil {
		return fmt.Errorf("failed to get secret: %v", err)
	}

	services, err := c.getPodRelatedServices(cluster, pPod)
	if err != nil {
		return fmt.Errorf("failed to list services from cluster %s cache: %v", cluster, err)
	}

	if len(services) == 0 {
		return fmt.Errorf("service is not ready")
	}

	conversion.MutatePod(vPod, pPod, vSecret, pSecret, services)

	_, err = c.client.Pods(targetNamespace).Create(pPod)
	if errors.IsAlreadyExists(err) {
		klog.Infof("pod %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) getPodRelatedServices(cluster string, pPod *v1.Pod) ([]*v1.Service, error) {
	var services []*v1.Service
	list, err := c.informer.Services().Lister().Services(conversion.ToSuperMasterNamespace(cluster, metav1.NamespaceDefault)).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	services = append(services, list...)

	list, err = c.informer.Services().Lister().Services(pPod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}
	services = append(services, list...)

	return services, nil
}

func (c *controller) reconcilePodUpdate(cluster, namespace, name string, vPod *v1.Pod) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	pPod, err := c.podLister.Pods(targetNamespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			// if the pod on super master has been deleted and syncer has not
			// deleted virtual pod with 0 grace period second successfully.
			// we depends on periodic check to do gc.
			return nil
		}
		return err
	}

	if vPod.DeletionTimestamp != nil {
		if pPod.DeletionTimestamp != nil {
			// pPod is under deletion, waiting for UWS bock populate the pod status.
			return nil
		}
		deleteOptions := metav1.NewDeleteOptions(*vPod.DeletionGracePeriodSeconds)
		deleteOptions.Preconditions = metav1.NewUIDPreconditions(string(pPod.UID))
		err = c.client.Pods(targetNamespace).Delete(name, deleteOptions)
		if errors.IsNotFound(err) {
			return nil
		}
		return err
	}

	updatedPod := conversion.CheckPodEquality(pPod, vPod)
	if updatedPod != nil {
		pPod, err = c.client.Pods(targetNamespace).Update(updatedPod)
		if err != nil {
			return err
		}
	}

	// pod has been updated by tenant controller
	if !equality.Semantic.DeepEqual(vPod.Status, pPod.Status) {
		c.enqueuePod(pPod)
	}

	return nil
}

func (c *controller) reconcilePodRemove(cluster, namespace, name string, vPod *v1.Pod) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &constants.DefaultDeletionPolicy,
	}
	err := c.client.Pods(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("pod %s/%s of cluster (%s) is not found in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster ctrl.ClusterInterface) {
	klog.Infof("tenant-masters-pod-controller watch cluster %s for pod resource", cluster.GetClusterName())
	err := c.multiClusterPodController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s pod event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster ctrl.ClusterInterface) {
	klog.Infof("tenant-masters-pod-controller stop watching cluster %s for pod resource", cluster.GetClusterName())
	c.multiClusterPodController.TeardownClusterResource(cluster)
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}
