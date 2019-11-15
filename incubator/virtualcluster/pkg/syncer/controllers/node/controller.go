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

package node

import (
	"sync"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/cluster"
	sc "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/controller"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/listener"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	"github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
)

type controller struct {
	sync.Mutex
	// map node name in super master to tenant cluster name it belongs to.
	nodeNameToCluster          map[string]map[string]struct{}
	nodeClient                 v1core.NodesGetter
	multiClusterNodeController *sc.MultiClusterController
}

func Register(
	client v1core.NodesGetter,
	nodeInformer coreinformers.NodeInformer,
	controllerManager *manager.ControllerManager,
) {
	c := &controller{
		nodeNameToCluster: make(map[string]map[string]struct{}),
		nodeClient:        client,
	}

	// Create the multi cluster node controller
	options := sc.Options{Reconciler: c}
	multiClusterNodeController, err := sc.NewController("tenant-masters-node-controller", &v1.Node{}, options)
	if err != nil {
		klog.Errorf("failed to create multi cluster pod controller %v", err)
		return
	}
	c.multiClusterNodeController = multiClusterNodeController
	controllerManager.AddController(multiClusterNodeController)

	nodeInformer.Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc: c.backPopulate,
			UpdateFunc: func(oldObj, newObj interface{}) {
				newNode := newObj.(*v1.Node)
				oldNode := oldObj.(*v1.Node)
				if newNode.ResourceVersion == oldNode.ResourceVersion {
					return
				}

				c.backPopulate(newObj)
			},
		},
	)

	// Register the controller as cluster change listener
	listener.AddListener(c)
}

func (c *controller) backPopulate(obj interface{}) {
	node := obj.(*v1.Node)

	klog.Infof("back populate node %s/%s", node.Name, node.Namespace)
	c.Lock()
	clusterList := c.nodeNameToCluster[node.Name]
	c.Unlock()

	if len(clusterList) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(clusterList))
	for clusterName, _ := range clusterList {
		c.updateClusterNodeStatus(clusterName, node, &wg)
	}
	wg.Wait()
}

func (c *controller) updateClusterNodeStatus(cluster string, node *v1.Node, wg *sync.WaitGroup) {
	defer wg.Done()

	innerCluster := c.multiClusterNodeController.GetCluster(cluster)
	client, err := clientset.NewForConfig(restclient.AddUserAgent(innerCluster.GetClientInfo().Config, "syncer"))
	if err != nil {
		klog.Errorf("could not find cluster %s in controller cache %v", cluster, err)
		return
	}

	vNode, err := client.CoreV1().Nodes().Get(node.Name, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("could not find node %s/%s: %v", cluster, node.Name, err)
		return
	}

	newVNode := vNode.DeepCopy()
	newVNode.Status.Conditions = node.Status.Conditions

	_, _, err = patchNodeStatus(client.CoreV1().Nodes(), types.NodeName(node.Name), vNode, newVNode)
	if err != nil {
		klog.Errorf("failed to update node %s/%s's heartbeats: %v", cluster, node.Name, err)
		return
	}
}

func (c *controller) Reconcile(request reconciler.Request) (reconciler.Result, error) {
	klog.Infof("reconcile node %s/%s %s event for cluster %s", request.Namespace, request.Name, request.Event, request.Cluster.Name)

	switch request.Event {
	case reconciler.AddEvent:
		err := c.reconcileCreate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Node))
		if err != nil {
			klog.Errorf("failed reconcile node %s/%s in cluster %s as %v", request.Namespace, request.Name, request.Cluster.Name, err)
			return reconciler.Result{Requeue: true}, err
		}
	case reconciler.UpdateEvent:
		err := c.reconcileUpdate(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Node))
		if err != nil {
			return reconciler.Result{}, err
		}
	case reconciler.DeleteEvent:
		err := c.reconcileRemove(request.Cluster.Name, request.Namespace, request.Name, request.Obj.(*v1.Node))
		if err != nil {
			return reconciler.Result{}, err
		}
	}
	return reconciler.Result{}, nil
}

func (c *controller) reconcileCreate(cluster, namespace, name string, node *v1.Node) error {
	c.Lock()
	defer c.Unlock()

	if _, exist := c.nodeNameToCluster[name]; !exist {
		c.nodeNameToCluster[name] = make(map[string]struct{})
	}
	c.nodeNameToCluster[name][cluster] = struct{}{}

	return nil
}

func (c *controller) reconcileUpdate(cluster, namespace, name string, node *v1.Node) error {
	c.Lock()
	defer c.Unlock()

	if _, exist := c.nodeNameToCluster[name]; !exist {
		c.nodeNameToCluster[name] = make(map[string]struct{})
	}
	c.nodeNameToCluster[name][cluster] = struct{}{}

	return nil
}

func (c *controller) reconcileRemove(cluster, namespace, name string, node *v1.Node) error {
	c.Lock()
	defer c.Unlock()

	if _, exists := c.nodeNameToCluster[name]; !exists {
		return nil
	}

	delete(c.nodeNameToCluster[name], cluster)

	return nil
}

func (c *controller) AddCluster(cluster *cluster.Cluster) {
	klog.Infof("tenant-masters-node-controller watch cluster %s for pod resource", cluster.Name)
	err := c.multiClusterNodeController.WatchClusterResource(cluster, sc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s node event: %v", cluster.Name, err)
	}
}

func (c *controller) RemoveCluster(cluster *cluster.Cluster) {
	klog.Warningf("not implemented yet")
}
