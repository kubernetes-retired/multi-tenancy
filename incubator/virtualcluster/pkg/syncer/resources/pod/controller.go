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
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/manager"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/reconciler"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
)

type controller struct {
	// syncer configuration
	config *config.SyncerConfiguration
	// super master pod client
	client v1core.CoreV1Interface
	// super master informer/listers/synced functions
	informer      coreinformers.Interface
	podLister     listersv1.PodLister
	podSynced     cache.InformerSynced
	serviceLister listersv1.ServiceLister
	serviceSynced cache.InformerSynced
	secretLister  listersv1.SecretLister
	secretSynced  cache.InformerSynced
	// Connect to all tenant master pod informers
	multiClusterPodController *mc.MultiClusterController
	// UWcontroller
	upwardPodController *uw.UpwardController
	// Periodic checker
	podPatroller *pa.Patroller
	// Cluster vNode PodMap and GCMap, needed for vNode garbage collection
	sync.Mutex
	clusterVNodePodMap map[string]map[string]map[string]struct{}
	clusterVNodeGCMap  map[string]map[string]VNodeGCStatus
	vNodeGCGracePeriod time.Duration
}

type VirtulNodeDeletionPhase string

const (
	VNodeQuiescing VirtulNodeDeletionPhase = "Quiescing"
	VNodeDeleting  VirtulNodeDeletionPhase = "Deleting"
)

type VNodeGCStatus struct {
	QuiesceStartTime metav1.Time
	Phase            VirtulNodeDeletionPhase
}

func NewPodController(config *config.SyncerConfiguration,
	client clientset.Interface,
	informer informers.SharedInformerFactory,
	vcClient vcclient.Interface,
	vcInformer vcinformers.VirtualClusterInformer,
	options *manager.ResourceSyncerOptions) (manager.ResourceSyncer, *mc.MultiClusterController, *uw.UpwardController, error) {
	c := &controller{
		config:             config,
		client:             client.CoreV1(),
		informer:           informer.Core().V1(),
		clusterVNodePodMap: make(map[string]map[string]map[string]struct{}),
		clusterVNodeGCMap:  make(map[string]map[string]VNodeGCStatus),
		vNodeGCGracePeriod: constants.DefaultvNodeGCGracePeriod,
	}
	var mcOptions *mc.Options
	if options == nil || options.MCOptions == nil {
		mcOptions = &mc.Options{Reconciler: c}
	} else {
		mcOptions = options.MCOptions
	}
	mcOptions.MaxConcurrentReconciles = constants.DwsControllerWorkerHigh
	multiClusterPodController, err := mc.NewMCController("tenant-masters-pod-controller", &v1.Pod{}, *mcOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create pod mc controller: %v", err)
	}
	c.multiClusterPodController = multiClusterPodController
	c.serviceLister = c.informer.Services().Lister()
	c.secretLister = c.informer.Secrets().Lister()
	c.podLister = c.informer.Pods().Lister()
	if options != nil && options.IsFake {
		c.serviceSynced = func() bool { return true }
		c.secretSynced = func() bool { return true }
		c.podSynced = func() bool { return true }
	} else {
		c.serviceSynced = c.informer.Services().Informer().HasSynced
		c.secretSynced = c.informer.Secrets().Informer().HasSynced
		c.podSynced = c.informer.Pods().Informer().HasSynced
	}
	var uwOptions *uw.Options
	if options == nil || options.UWOptions == nil {
		uwOptions = &uw.Options{Reconciler: c}
	} else {
		uwOptions = options.UWOptions
	}
	uwOptions.MaxConcurrentReconciles = constants.UwsControllerWorkerHigh
	upwardPodController, err := uw.NewUWController("pod-upward-controller", &v1.Pod{}, *uwOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create pod upward controller: %v", err)
	}
	c.upwardPodController = upwardPodController

	var patrolOptions *pa.Options
	if options == nil || options.PatrolOptions == nil {
		patrolOptions = &pa.Options{Reconciler: c}
	} else {
		patrolOptions = options.PatrolOptions
	}
	podPatroller, err := pa.NewPatroller("pod-patroller", &v1.Pod{}, *patrolOptions)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create pod patroller %v", err)
	}
	c.podPatroller = podPatroller

	c.informer.Pods().Informer().AddEventHandler(
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
	return c, multiClusterPodController, upwardPodController, nil
}

func (c *controller) enqueuePod(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		return
	}

	clusterName, _ := conversion.GetVirtualOwner(pod)
	if clusterName == "" {
		return
	}

	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("couldn't get key for object %v: %v", obj, err))
		return
	}

	c.upwardPodController.AddToQueue(key)
}

// c.Mutex needs to be Locked before calling addToClusterVNodeGCMap
func (c *controller) addToClusterVNodeGCMap(cluster string, nodeName string) {
	if _, exist := c.clusterVNodeGCMap[cluster]; !exist {
		c.clusterVNodeGCMap[cluster] = make(map[string]VNodeGCStatus)
	}
	c.clusterVNodeGCMap[cluster][nodeName] = VNodeGCStatus{
		QuiesceStartTime: metav1.Now(),
		Phase:            VNodeQuiescing,
	}
}

// c.Mutex needs to be Locked before calling removeQuiescingNodeFromClusterVNodeGCMap
func (c *controller) removeQuiescingNodeFromClusterVNodeGCMap(cluster string, nodeName string) bool {
	if _, exist := c.clusterVNodeGCMap[cluster]; exist {
		if _, exist := c.clusterVNodeGCMap[cluster][nodeName]; exist {
			if c.clusterVNodeGCMap[cluster][nodeName].Phase == VNodeQuiescing {
				delete(c.clusterVNodeGCMap[cluster], nodeName)
				return true
			} else {
				return false
			}
		}
	}
	return true
}

func (c *controller) checkClusterVNodePodMap(clusterName, nodeName, uid string) bool {
	c.Lock()
	defer c.Unlock()
	if _, exist := c.clusterVNodePodMap[clusterName]; !exist {
		return false
	} else if _, exist := c.clusterVNodePodMap[clusterName][nodeName]; !exist {
		return false
	} else if _, exist := c.clusterVNodePodMap[clusterName][nodeName][uid]; !exist {
		return false
	}
	return true
}

func (c *controller) updateClusterVNodePodMap(clusterName, nodeName, requestUID string, event reconciler.EventType) {
	c.Lock()
	defer c.Unlock()
	if event == reconciler.UpdateEvent {
		if _, exist := c.clusterVNodePodMap[clusterName]; !exist {
			c.clusterVNodePodMap[clusterName] = make(map[string]map[string]struct{})
		}
		if _, exist := c.clusterVNodePodMap[clusterName][nodeName]; !exist {
			c.clusterVNodePodMap[clusterName][nodeName] = make(map[string]struct{})
		}
		c.clusterVNodePodMap[clusterName][nodeName][requestUID] = struct{}{}
		if !c.removeQuiescingNodeFromClusterVNodeGCMap(clusterName, nodeName) {
			// We have consistency issue here. TODO: add to metrics
			klog.Errorf("Cluster %s has vPods in vNode %s which is being GCed!", clusterName, nodeName)
		}
	} else { // delete
		if _, exist := c.clusterVNodePodMap[clusterName][nodeName]; exist {
			if _, exist := c.clusterVNodePodMap[clusterName][nodeName][requestUID]; exist {
				delete(c.clusterVNodePodMap[clusterName][nodeName], requestUID)
			} else {
				klog.Warningf("Deleted pod %s of cluster (%s) is not found in clusterVNodePodMap", requestUID, clusterName)
			}

			// If vNode does not have any Pod left, put it into gc map
			if len(c.clusterVNodePodMap[clusterName][nodeName]) == 0 {
				c.addToClusterVNodeGCMap(clusterName, nodeName)
				delete(c.clusterVNodePodMap[clusterName], nodeName)
			}
		} else {
			klog.Warningf("The nodename %s of deleted pod %s in cluster (%s) is not found in clusterVNodePodMap", nodeName, requestUID, clusterName)
		}
	}
}

func (c *controller) AddCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-pod-controller watch cluster %s for pod resource", cluster.GetClusterName())
	err := c.multiClusterPodController.WatchClusterResource(cluster, mc.WatchOptions{})
	if err != nil {
		klog.Errorf("failed to watch cluster %s pod event: %v", cluster.GetClusterName(), err)
	}
}

func (c *controller) RemoveCluster(cluster mc.ClusterInterface) {
	klog.Infof("tenant-masters-pod-controller stop watching cluster %s for pod resource", cluster.GetClusterName())
	c.multiClusterPodController.TeardownClusterResource(cluster)
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}
