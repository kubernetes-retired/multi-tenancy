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

package scheduler

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/apis/cluster/v1alpha4"
	superListers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/listers/cluster/v1alpha4"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vcListers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/conversion"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/cluster"
	utilconst "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
	mc "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/mccontroller"
)

type virtualclusterGetter struct {
	lister vcListers.VirtualClusterLister
}

var _ mc.Getter = &virtualclusterGetter{}

func (v *virtualclusterGetter) GetObject(namespace, name string) (runtime.Object, error) {
	vc, err := v.lister.VirtualClusters(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	return vc, nil
}

func (s *Scheduler) virtualClusterWorkerRun() {
	for s.processNextVirtualClusterItem() {
	}
}

func (s *Scheduler) processNextVirtualClusterItem() bool {
	key, quit := s.virtualClusterQueue.Get()
	if quit {
		return false
	}
	defer s.virtualClusterQueue.Done(key)

	err := s.syncVirtualCluster(key.(string))
	if err == nil {
		s.virtualClusterQueue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing virtual cluster %v (will retry): %v", key, err))
	s.virtualClusterQueue.AddRateLimited(key)
	return true
}

func (s *Scheduler) syncVirtualCluster(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	vc, err := s.virtualClusterLister.VirtualClusters(namespace).Get(name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		s.removeVirtualCluster(key)
		return nil
	}

	switch vc.Status.Phase {
	case v1alpha1.ClusterRunning:
		return s.addVirtualCluster(key, vc)
	case v1alpha1.ClusterError:
		s.removeVirtualCluster(key)
		return nil
	default:
		klog.Infof("Virtual cluster %s/%s not ready to reconcile", vc.Namespace, vc.Name)
		return nil
	}
}

func (s *Scheduler) removeVirtualCluster(key string) {
	klog.Infof("Remove virtualcluster %s", key)

	s.virtualClusterLock.Lock()
	defer s.virtualClusterLock.Unlock()

	vc, exist := s.virtualClusterSet[key]
	if !exist {
		// already deleted
		return
	}

	vc.Stop()

	for _, clusterChangeListener := range s.virtualClusterWatcher.GetListeners() {
		clusterChangeListener.RemoveCluster(vc)
	}

	delete(s.virtualClusterSet, key)
}

func (s *Scheduler) addVirtualCluster(key string, vc *v1alpha1.VirtualCluster) error {
	klog.Infof("Add virtualcluster %s", key)

	s.virtualClusterLock.Lock()
	if _, exist := s.virtualClusterSet[key]; exist {
		s.virtualClusterLock.Unlock()
		return nil
	}
	s.virtualClusterLock.Unlock()

	clusterName := conversion.ToClusterKey(vc)

	adminKubeConfigBytes, err := conversion.GetKubeConfigOfVC(s.metaClusterClient.CoreV1(), vc)
	if err != nil {
		return err
	}

	tenantCluster, err := cluster.NewCluster(clusterName, vc.Namespace, vc.Name, string(vc.UID), &virtualclusterGetter{lister: s.virtualClusterLister}, adminKubeConfigBytes, cluster.Options{})
	if err != nil {
		return fmt.Errorf("failed to new tenant cluster %s/%s: %v", vc.Namespace, vc.Name, err)
	}

	// for each resource type of the newly added VirtualCluster, we add a listener
	for _, clusterChangeListener := range s.virtualClusterWatcher.GetListeners() {
		clusterChangeListener.AddCluster(tenantCluster)
	}

	s.virtualClusterLock.Lock()
	s.virtualClusterSet[key] = tenantCluster
	s.virtualClusterLock.Unlock()

	go s.syncVirtualClusterCache(tenantCluster, vc)

	return nil
}

func (s *Scheduler) syncVirtualClusterCache(cluster *cluster.Cluster, vc *v1alpha1.VirtualCluster) {
	go func() {
		err := cluster.Start()
		klog.Infof("cluster %s shutdown: %v", cluster.GetClusterName(), err)
	}()

	if !cluster.WaitForCacheSync() {
		s.recorder.Eventf(&v1.ObjectReference{
			Kind:      "VirtualCluster",
			Namespace: vc.Namespace,
			Name:      vc.Name,
			UID:       vc.UID,
		}, v1.EventTypeWarning, "ClusterUnHealth", "VirtualCluster %v unhealth: failed to sync cache", cluster.GetClusterName())

		klog.Warningf("failed to sync cache for virtualcluster %s, retry", cluster.GetClusterName())
		key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(vc)
		s.removeVirtualCluster(key)
		s.virtualClusterQueue.AddAfter(key, 5*time.Second)
		return
	}
	cluster.SetSynced()
}

type superclusterGetter struct {
	lister superListers.ClusterLister
}

var _ mc.Getter = &superclusterGetter{}

func (v *superclusterGetter) GetObject(namespace, name string) (runtime.Object, error) {
	super, err := v.lister.Clusters(namespace).Get(name)
	if err != nil {
		return nil, err
	}
	return super, nil
}

func (s *Scheduler) superClusterWorkerRun() {
	for s.processNextSuperClusterItem() {
	}
}

func (s *Scheduler) processNextSuperClusterItem() bool {
	key, quit := s.superClusterQueue.Get()
	if quit {
		return false
	}
	defer s.superClusterQueue.Done(key)

	err := s.syncSuperCluster(key.(string))
	if err == nil {
		s.superClusterQueue.Forget(key)
		return true
	}

	utilruntime.HandleError(fmt.Errorf("error processing super cluster %v (will retry): %v", key, err))
	s.superClusterQueue.AddRateLimited(key)
	return true
}

func (s *Scheduler) syncSuperCluster(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	super, err := s.superClusterLister.Clusters(namespace).Get(name)
	if err != nil {
		if !errors.IsNotFound(err) {
			return err
		}

		s.removeSuperCluster(key)
		return nil
	}

	switch v1alpha4.ClusterPhase(super.Status.Phase) {
	case v1alpha4.ClusterPhaseProvisioned:
		return s.addSuperCluster(key, super)
	case v1alpha4.ClusterPhaseFailed:
		s.removeSuperCluster(key)
		return nil
	default:
		klog.Infof("SuperCluster %s/%s not ready to reconcile", super.Namespace, super.Name)
		return nil
	}
}

func (s *Scheduler) removeSuperCluster(key string) {
	klog.Infof("Remove supercluster %s", key)

	s.superClusterLock.Lock()
	defer s.superClusterLock.Unlock()

	super, exist := s.superClusterSet[key]
	if !exist {
		// already deleted
		return
	}

	super.Stop()
	for _, clusterChangeListener := range s.superClusterWatcher.GetListeners() {
		clusterChangeListener.RemoveCluster(super)
	}

	delete(s.superClusterSet, key)
}

func (s *Scheduler) addSuperCluster(key string, super *v1alpha4.Cluster) error {
	klog.Infof("Add supercluster %s", key)

	s.superClusterLock.Lock()
	if _, exist := s.superClusterSet[key]; exist {
		s.superClusterLock.Unlock()
		return nil
	}
	s.superClusterLock.Unlock()

	clusterName := fmt.Sprintf("%s/%s", super.Namespace, super.Name)

	// we assume the super cluster kubeconfig is saved in a secret with the same name of the cluster CR in the same namespace.
	// this may change in the future
	adminKubeConfigSecret, err := s.metaClusterClient.CoreV1().Secrets(super.Namespace).Get(context.TODO(), super.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret (%s) for super cluster in namespace %s: %v", super.Name, super.Namespace, err)
	}
	adminKubeConfigBytes := adminKubeConfigSecret.Data[constants.KubeconfigAdminSecretName]

	superCluster, err := cluster.NewCluster(clusterName, super.Namespace, super.Name, string(super.UID), &superclusterGetter{lister: s.superClusterLister}, adminKubeConfigBytes, cluster.Options{})
	if err != nil {
		return fmt.Errorf("failed to new super cluster %s/%s: %v", super.Namespace, super.Name, err)
	}
	// the super cluster should have the id configmap in kube-system
	cs, _ := superCluster.GetClientSet()
	cfg, err := cs.CoreV1().ConfigMaps("kube-system").Get(context.TODO(), utilconst.SuperClusterInfoCfgMap, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get super cluster info configmap in kube-system")
	}
	id, ok := cfg.Data[utilconst.SuperClusterIDKey]
	if !ok {
		return fmt.Errorf("failed to get super cluster id from the supercluster-info configmap in kube-system")
	}
	klog.Infof("supercluster %s's ID is found: %v", key, id)

	// for each resource type of the newly added VirtualCluster, we add a listener
	for _, clusterChangeListener := range s.superClusterWatcher.GetListeners() {
		clusterChangeListener.AddCluster(superCluster)
	}

	s.superClusterLock.Lock()
	s.superClusterSet[key] = superCluster
	s.superClusterLock.Unlock()

	go s.syncSuperClusterCache(superCluster, super)

	return nil
}

func (s *Scheduler) syncSuperClusterCache(cluster *cluster.Cluster, super *v1alpha4.Cluster) {
	go func() {
		err := cluster.Start()
		klog.Infof("supercluster %s shutdown: %v", cluster.GetClusterName(), err)
	}()

	if !cluster.WaitForCacheSync() {
		s.recorder.Eventf(&v1.ObjectReference{
			Kind:      "Cluster",
			Namespace: super.Namespace,
			Name:      super.Name,
			UID:       super.UID,
		}, v1.EventTypeWarning, "ClusterUnHealth", "SuperCluster %v unhealth: failed to sync cache", cluster.GetClusterName())

		klog.Warningf("failed to sync cache for supercluster %s, retry", cluster.GetClusterName())
		key, _ := cache.DeletionHandlingMetaNamespaceKeyFunc(super)
		s.removeSuperCluster(key)
		s.superClusterQueue.AddAfter(key, 5*time.Second)
		return
	}
	cluster.SetSynced()
}
