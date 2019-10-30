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

	"k8s.io/api/core/v1"
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
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type controller struct {
	podClient                 v1core.PodsGetter
	multiClusterPodController *sc.MultiClusterController
}

func Register(
	podClient v1core.PodsGetter,
	podInformer coreinformers.PodInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		podClient: podClient,
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
	vPodObj, err := c.multiClusterPodController.Get(clusterName, namespace, pod.Name)
	if err != nil {
		return
	}
	var client *clientset.Clientset
	vPod := vPodObj.(*v1.Pod)
	if vPod.Spec.NodeName != pod.Spec.NodeName {
		innerCluster := c.multiClusterPodController.GetCluster(clusterName)
		client, err = clientset.NewForConfig(restclient.AddUserAgent(innerCluster.GetClientInfo().Config, "syncer"))
		if err != nil {
			return
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
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcilePodUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Pod))
		if err != nil {
			return reconciler.Result{}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcilePodRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Pod))
		if err != nil {
			return reconciler.Result{}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcilePodCreate(cluster, namespace, name string, pod *v1.Pod) error {
	targetNamespace := conversion.ToSuperMasterNamespace(cluster, namespace)
	newObj, err := conversion.BuildMetadata(targetNamespace, pod)
	if err != nil {
		return err
	}

	pPod := newObj.(*v1.Pod)
	conversion.MutatePod(targetNamespace, pPod)

	innerCluster := c.multiClusterPodController.GetCluster(cluster)
	client, err := clientset.NewForConfig(restclient.AddUserAgent(innerCluster.GetClientInfo().Config, "syncer"))
	if err != nil {
		return err
	}
	_, err = client.CoreV1().Pods(targetNamespace).Create(pod)
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
	err := c.podClient.Pods(targetNamespace).Delete(name, opts)
	if errors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *controller) AddCluster(cluster *cluster.Cluster) {
	err := c.multiClusterPodController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s pod event", cluster.Name)
	}
}

func (c *controller) RemoveCluster(cluster *cluster.Cluster) {
	klog.Warningf("not implemented yet")
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}
