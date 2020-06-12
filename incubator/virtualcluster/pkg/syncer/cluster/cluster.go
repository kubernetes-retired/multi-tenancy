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

package cluster

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	clientgocache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
	vclisters "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/listers/tenancy/v1alpha1"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/constants"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller"
)

// Each Cluster object represents a tenant master in Virtual Cluster architecture.
//
// Cluster implements the ClusterInterface used by MultiClusterController in
// sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/mccontroller/mccontroller.go.
//
// It stores a Kubernetes client, cache, and other cluster-scoped dependencies.
// The dependencies are lazily created in getters and cached for reuse.
// It is not thread safe.
type Cluster struct {
	// The root namespace name for this cluster
	key string
	// Name of the corresponding virtual cluster object.
	vcName string
	// Namespace of the corresponding virtual cluster object.
	vcNamespace string
	// UID of the corresponding virtual cluster object.
	vcUID string

	// Config is the rest.config used to talk to the apiserver.  Required.
	RestConfig *rest.Config

	// vcLister points to the super master virtual cluster informer cache.
	vclister vclisters.VirtualClusterLister

	// scheme is injected by the controllerManager when controllerManager.Start is called
	scheme *runtime.Scheme

	mapper meta.RESTMapper

	// informer cache and delegating client for watched tenant master objects
	cache            cache.Cache
	delegatingClient *client.DelegatingClient

	// a clientset client for unwatched tenant master objects (rw directly to tenant apiserver)
	client *clientset.Clientset

	Options

	// a flag indicates that the cluster cache has been synced
	synced bool

	stopCh chan struct{}
}

// Options are the arguments for creating a new Cluster.
type Options struct {
	CacheOptions
}

// CacheOptions is embedded in Options to configure the new Cluster's cache.
type CacheOptions struct {
	// Resync is the period between cache resyncs.
	// A cache resync triggers event handlers for each object watched by the cache.
	// It can be useful if your level-based logic isn't perfect.
	Resync *time.Duration
	// Namespace can be used to watch only a single namespace.
	// If unset (Namespace == ""), all namespaces are watched.
	Namespace string
}

var _ mccontroller.ClusterInterface = &Cluster{}

// New creates a new Cluster.
func NewTenantCluster(key, namespace, name, uid string, vclister vclisters.VirtualClusterLister, configBytes []byte, o Options) (*Cluster, error) {
	clusterRestConfig, err := clientcmd.RESTConfigFromKubeConfig(configBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to build rest config: %v", err)
	}

	if clusterRestConfig.QPS == 0 {
		clusterRestConfig.QPS = constants.DefaultSyncerClientQPS
	}
	if clusterRestConfig.Burst == 0 {
		clusterRestConfig.Burst = constants.DefaultSyncerClientBurst
	}

	return &Cluster{
		key:         key,
		vcName:      name,
		vcNamespace: namespace,
		vcUID:       uid,
		vclister:    vclister,
		RestConfig:  clusterRestConfig,
		Options:     o,
		synced:      false,
		stopCh:      make(chan struct{})}, nil
}

// GetClusterName returns the unique cluster name, aka, the root namespace name.
func (c *Cluster) GetClusterName() string {
	return c.key
}

func (c *Cluster) GetOwnerInfo() (string, string, string) {
	return c.vcName, c.vcNamespace, c.vcUID
}

// GetSpec returns the virtual cluster spec.
func (c *Cluster) GetSpec() (*v1alpha1.VirtualClusterSpec, error) {
	vc, err := c.vclister.VirtualClusters(c.vcNamespace).Get(c.vcName)
	if err != nil {
		return nil, err
	}

	spec := vc.Spec.DeepCopy()
	prefixesSet := sets.NewString(spec.OpaqueMetaPrefixes...)
	if !prefixesSet.Has(constants.DefaultOpaqueMetaPrefix) {
		spec.OpaqueMetaPrefixes = append(spec.OpaqueMetaPrefixes, constants.DefaultOpaqueMetaPrefix)
	}
	prefixesSet = sets.NewString(spec.TransparentMetaPrefixes...)
	if !prefixesSet.Has(constants.DefaultTransparentMetaPrefix) {
		spec.TransparentMetaPrefixes = append(spec.TransparentMetaPrefixes, constants.DefaultTransparentMetaPrefix)
	}

	return spec, nil
}

func (c *Cluster) getScheme() *runtime.Scheme {
	return scheme.Scheme
}

// GetClientSet returns a clientset client without any informer caches. All client requests go to apiserver directly.
func (c *Cluster) GetClientSet() (clientset.Interface, error) {
	if c.client != nil {
		return c.client, nil
	}
	var err error
	c.client, err = clientset.NewForConfig(restclient.AddUserAgent(c.RestConfig, constants.ResourceSyncerUserAgent))
	if err != nil {
		return nil, err
	}
	return c.client, nil
}

// getMapper returns a lazily created apimachinery RESTMapper.
func (c *Cluster) getMapper() (meta.RESTMapper, error) {
	if c.mapper != nil {
		return c.mapper, nil
	}

	mapper, err := apiutil.NewDiscoveryRESTMapper(c.RestConfig)
	if err != nil {
		return nil, err
	}

	c.mapper = mapper
	return mapper, nil
}

// getCache returns a lazily created controller-runtime Cache.
func (c *Cluster) getCache() (cache.Cache, error) {
	if c.cache != nil {
		return c.cache, nil
	}

	m, err := c.getMapper()
	if err != nil {
		return nil, err
	}

	ca, err := cache.New(c.RestConfig, cache.Options{
		Scheme:    c.getScheme(),
		Mapper:    m,
		Resync:    c.Resync,
		Namespace: c.Namespace,
	})
	if err != nil {
		return nil, err
	}

	c.cache = ca
	return ca, nil
}

// GetDelegatingClient returns a lazily created controller-runtime DelegatingClient.
// It is used by other Cluster getters, and by reconcilers.
// TODO: consider implementing Reader, Writer and StatusClient in Cluster
// and forwarding to actual delegating client.
func (c *Cluster) GetDelegatingClient() (client.Client, error) {
	if !c.synced {
		return nil, fmt.Errorf("The client cache has not been synced yet.")
	}

	if c.delegatingClient != nil {
		return c.delegatingClient, nil
	}

	ca, err := c.getCache()
	if err != nil {
		return nil, err
	}

	m, err := c.getMapper()
	if err != nil {
		return nil, err
	}

	cl, err := client.New(c.RestConfig, client.Options{
		Scheme: c.getScheme(),
		Mapper: m,
	})
	if err != nil {
		return nil, err
	}

	dc := &client.DelegatingClient{
		Reader: &client.DelegatingReader{
			CacheReader:  ca,
			ClientReader: cl,
		},
		Writer:       cl,
		StatusClient: cl,
	}

	c.delegatingClient = dc
	return dc, nil
}

// AddEventHandler instructs the Cluster's cache to watch objectType's resource,
// if it doesn't already, and to add handler as an event handler.
func (c *Cluster) AddEventHandler(objectType runtime.Object, handler clientgocache.ResourceEventHandler) error {
	ca, err := c.getCache()
	if err != nil {
		return err
	}

	i, err := ca.GetInformer(objectType)
	if err != nil {
		return err
	}

	i.AddEventHandler(handler)
	return nil
}

// Start starts the Cluster's cache and blocks,
// until an empty struct is sent to the stop channel.
func (c *Cluster) Start() error {
	ca, err := c.getCache()
	if err != nil {
		return err
	}
	return ca.Start(c.stopCh)
}

// WaitForCacheSync waits for the Cluster's cache to sync,
// OR until an empty struct is sent to the stop channel.
func (c *Cluster) WaitForCacheSync() bool {
	ca, err := c.getCache()
	if err != nil {
		klog.Errorf("Fail to get cache: %v", err)
		return false
	}
	return ca.WaitForCacheSync(c.stopCh)
}

func (c *Cluster) SetSynced() {
	c.synced = true
}

// Stop send a msg to stopCh, stop the cache.
func (c *Cluster) Stop() {
	close(c.stopCh)
}
