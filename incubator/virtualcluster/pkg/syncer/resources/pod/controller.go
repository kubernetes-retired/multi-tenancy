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
	pa "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/patrol"
	uw "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/uwcontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/vnode/provider"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/plugin"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/reconciler"
)

func init() {
	plugin.SyncerResourceRegister.Register(&plugin.Registration{
		ID: "pod",
		InitFn: func(ctx *plugin.InitContext) (interface{}, error) {
			return NewPodController(ctx.Config.(*config.SyncerConfiguration), ctx.Client, ctx.Informer, ctx.VCClient, ctx.VCInformer, manager.ResourceSyncerOptions{})
		},
	})
}

type controller struct {
	manager.BaseResourceSyncer
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
	// Cluster vNode PodMap and GCMap, needed for vNode garbage collection
	sync.Mutex
	clusterVNodePodMap map[string]map[string]map[string]struct{}
	clusterVNodeGCMap  map[string]map[string]VNodeGCStatus
	vNodeGCGracePeriod time.Duration
	// vnodeProvider manages vnode object.
	vnodeProvider provider.VirtualNodeProvider
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
	options manager.ResourceSyncerOptions) (manager.ResourceSyncer, error) {
	c := &controller{
		BaseResourceSyncer: manager.BaseResourceSyncer{
			Config: config,
		},
		client:             client.CoreV1(),
		informer:           informer.Core().V1(),
		clusterVNodePodMap: make(map[string]map[string]map[string]struct{}),
		clusterVNodeGCMap:  make(map[string]map[string]VNodeGCStatus),
		vNodeGCGracePeriod: constants.DefaultvNodeGCGracePeriod,
		vnodeProvider:      vnode.GetNodeProvider(config, client),
	}

	var err error
	c.MultiClusterController, err = mc.NewMCController(&v1.Pod{}, &v1.PodList{}, c,
		mc.WithMaxConcurrentReconciles(constants.DwsControllerWorkerHigh), mc.WithOptions(options.MCOptions))
	if err != nil {
		return nil, err
	}

	c.serviceLister = c.informer.Services().Lister()
	c.secretLister = c.informer.Secrets().Lister()
	c.podLister = c.informer.Pods().Lister()
	if options.IsFake {
		c.serviceSynced = func() bool { return true }
		c.secretSynced = func() bool { return true }
		c.podSynced = func() bool { return true }
	} else {
		c.serviceSynced = c.informer.Services().Informer().HasSynced
		c.secretSynced = c.informer.Secrets().Informer().HasSynced
		c.podSynced = c.informer.Pods().Informer().HasSynced
	}

	c.UpwardController, err = uw.NewUWController(&v1.Pod{}, c,
		uw.WithMaxConcurrentReconciles(constants.UwsControllerWorkerHigh), uw.WithOptions(options.UWOptions))
	if err != nil {
		return nil, err
	}

	c.Patroller, err = pa.NewPatroller(&v1.Pod{}, c, pa.WithOptions(options.PatrolOptions))
	if err != nil {
		return nil, err
	}

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
	return c, nil
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

	c.UpwardController.AddToQueue(key)
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

func (c *controller) SetVNodeProvider(provider provider.VirtualNodeProvider) {
	c.Lock()
	c.vnodeProvider = provider
	c.Unlock()
}

// assignedPod selects pods that are assigned (scheduled and running).
func assignedPod(pod *v1.Pod) bool {
	return len(pod.Spec.NodeName) != 0
}
