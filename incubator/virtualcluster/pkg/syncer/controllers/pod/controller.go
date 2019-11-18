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
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controllers/node"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/utils"
)

type controller struct {
	client                    v1core.CoreV1Interface
	multiClusterPodController *sc.MultiClusterController
}

func Register(
	client v1core.CoreV1Interface,
	podInformer coreinformers.PodInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		client: client,
	}

	// Create the multi cluster pod controller
	options := sc.Options{Reconciler: c}
	multiClusterPodController, err := sc.NewController("tenant-masters-pod-controller", &v1.Pod{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster pod controller %v", err)
		return
	}
	c.multiClusterPodController = multiClusterPodController
	controllerManager.AddController(multiClusterPodController)

	podInformer.Informer().AddEventHandler(
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
				AddFunc: c.backPopulate,
				UpdateFunc: func(oldObj, newObj interface{}) {
					newPod := newObj.(*v1.Pod)
					oldPod := oldObj.(*v1.Pod)
					if newPod.ResourceVersion == oldPod.ResourceVersion {
						// Periodic resync will send update events for all known Deployments.
						// Two different versions of the same Deployment will always have different RVs.
						return
					}

					c.backPopulate(newObj)
				},
			},
		},
	)

	// Register the controller as cluster change listener
	listener.AddListener(c)
}

func (c *controller) backPopulate(obj interface{}) {
	pod := obj.(*v1.Pod)
	clusterName, namespace := conversion.GetOwner(pod)
	if len(clusterName) == 0 {
		return
	}
	klog.Infof("back populate pod %s/%s in cluster %s", pod.Name, namespace, clusterName)
	vPodObj, err := c.multiClusterPodController.Get(clusterName, namespace, pod.Name)
	if err != nil {
		klog.Errorf("could not find pod %s/%s pod in controller cache %v", pod.Name, namespace, err)
		return
	}
	var client *clientset.Clientset
	innerCluster := c.multiClusterPodController.GetCluster(clusterName)
	client, err = clientset.NewForConfig(restclient.AddUserAgent(innerCluster.GetClientInfo().Config, "syncer"))
	if err != nil {
		return
	}

	vPod := vPodObj.(*v1.Pod)
	if vPod.Spec.NodeName != pod.Spec.NodeName {
		n, err := c.client.Nodes().Get(pod.Spec.NodeName, metav1.GetOptions{})
		if err != nil {
			klog.Errorf("failed to get node %s from super master: %v", pod.Spec.NodeName, err)
			return
		}

		_, err = client.CoreV1().Nodes().Create(node.NewVirtualNode(n))
		if errors.IsAlreadyExists(err) {
			klog.Warningf("virtual node %s already exists", vPod.Spec.NodeName)
		}

		err = client.CoreV1().Pods(vPod.Namespace).Bind(&v1.Binding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      vPod.Name,
				Namespace: vPod.Namespace,
			},
			Target: v1.ObjectReference{
				Kind:       "Node",
				Name:       pod.Spec.NodeName,
				APIVersion: "v1",
			},
		})
		if err != nil {
			klog.Errorf("failed to bind vPod %s/%s to node %s %v", vPod.Namespace, vPod.Name, pod.Spec.NodeName, err)
		}
		return
	}
	if !equality.Semantic.DeepEqual(vPod.Status, pod.Status) {
		newPod := vPod.DeepCopy()
		newPod.Status = pod.Status
		if _, err = client.CoreV1().Pods(vPod.Namespace).UpdateStatus(newPod); err != nil {
			klog.Errorf("failed to back populate pod %s/%s status update for cluster %s: %v", vPod.Namespace, vPod.Name, clusterName, err)
		}
	}
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

func (c *controller) reconcilePodCreate(cluster, namespace, name string, pod *v1.Pod) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	newObj, err := conversion.BuildMetadata(cluster, targetNamespace, pod)
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

	secret, err := utils.GetSecret(c.client, targetNamespace, saName)
	if err != nil {
		return fmt.Errorf("failed to get secret: %v", err)
	}

	if secret.Labels[conversion.SecretSyncStatusKey] != conversion.SecretSyncStatusReady {
		return fmt.Errorf("secret for pod is not ready")
	}

	var client *clientset.Clientset
	innerCluster := c.multiClusterPodController.GetCluster(cluster)
	client, err = clientset.NewForConfig(restclient.AddUserAgent(innerCluster.GetClientInfo().Config, "syncer"))
	if err != nil {
		return err
	}
	vSecret, err := utils.GetSecret(client.CoreV1(), namespace, saName)
	if err != nil {
		return fmt.Errorf("failed to get secret: %v", err)
	}

	// list service in tenant ns and inject them into the pod
	ca, err := innerCluster.GetCache()
	if err != nil {
		return fmt.Errorf("failed to get cluster %s cache: %v", cluster, err)
	}
	services := v1.ServiceList{}
	err = ca.List(context.Background(), &services)
	if err != nil {
		return fmt.Errorf("failed to list services from cluster %s cache: %v", cluster, err)
	}

	conversion.MutatePod(targetNamespace, pPod, vSecret, secret, services.Items)

	_, err = c.client.Pods(targetNamespace).Create(pPod)
	if errors.IsAlreadyExists(err) {
		klog.Infof("pod %s/%s of cluster %s already exist in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) reconcilePodUpdate(cluster, namespace, name string, pod *v1.Pod) error {
	return nil
}

func (c *controller) reconcilePodRemove(cluster, namespace, name string, pod *v1.Pod) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	opts := &metav1.DeleteOptions{
		PropagationPolicy: &conversion.DefaultDeletionPolicy,
	}
	err := c.client.Pods(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		klog.Warningf("pod %s/%s of cluster (%s) is not found in super master", namespace, name, cluster)
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster *cluster.Cluster) {
	klog.Infof("tenant-masters-pod-controller watch cluster %s for pod resource", cluster.Name)
	err := c.multiClusterPodController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s pod event: %v", cluster.Name, err)
	}
}

func (c *controller) RemoveCluster(cluster *cluster.Cluster) {
	klog.Warningf("not implemented yet")
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}
