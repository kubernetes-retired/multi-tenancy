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

package options

import (
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/util/uuid"
	corev1 "k8s.io/api/core/v1"
	componentbaseconfig "k8s.io/component-base/config"
	cliflag "k8s.io/component-base/cli/flag"
	clientset "k8s.io/client-go/kubernetes"
	clientgokubescheme "k8s.io/client-go/kubernetes/scheme"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"k8s.io/klog"

	syncerconfig "github.com/multi-tenancy/incubator/virtualcluster/cmd/syncer/app/config"
	vcinformers "github.com/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions"
	vcclient "github.com/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
)

const (
	// ResourceSyncerUserAgent is the userAgent name when starting resource syncer.
	ResourceSyncerUserAgent = "resource-syncer"
)

// ResourceSyncerOptions is the main context object for the resource syncer.
type ResourceSyncerOptions struct {
	// ClientConnection specifies the kubeconfig file and client connection
	// settings for the proxy server to use when communicating with the apiserver.
	ClientConnection componentbaseconfig.ClientConnectionConfiguration
	// leaderElection defines the configuration of leader election client.
	LeaderElection componentbaseconfig.LeaderElectionConfiguration

	SuperMaster           string
	SuperMasterKubeconfig string
}

// NewResourceSyncerOptions creates a new resource syncer with a default config.
func NewResourceSyncerOptions() (*ResourceSyncerOptions, error) {
	return &ResourceSyncerOptions{}, nil
}

func (o *ResourceSyncerOptions) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}

	fs := fss.FlagSet("misc")
	fs.StringVar(&o.SuperMaster, "super-master", o.SuperMaster, "The address of the super master Kubernetes API server (overrides any value in super-master-kubeconfig).")
	fs.StringVar(&o.SuperMasterKubeconfig, "super-master-kubeconfig", o.SuperMasterKubeconfig, "Path to kubeconfig file with authorization and master location information.")

	return fss
}

// Config return a syncer config object
func (o *ResourceSyncerOptions) Config() (*syncerconfig.Config, error) {
	c := &syncerconfig.Config{}

	// Prepare kube clients
	inClusterClient, leaderElectionClient, virtualClusterClient, superMasterClient, err := createClients(o.ClientConnection, o.SuperMaster, o.LeaderElection.RenewDeadline.Duration)
	if err != nil {
		return nil, err
	}

	// Prepare event clients.
	eventBroadcaster := events.NewBroadcaster(&events.EventSinkImpl{Interface: superMasterClient.EventsV1beta1().Events("")})
	recorder := eventBroadcaster.NewRecorder(clientgokubescheme.Scheme, ResourceSyncerUserAgent)
	leaderElectionBroadcaster := record.NewBroadcaster()
	leaderElectionRecorder := leaderElectionBroadcaster.NewRecorder(clientgokubescheme.Scheme, corev1.EventSource{Component: ResourceSyncerUserAgent})

	// Set up leader election if enabled.
	var leaderElectionConfig *leaderelection.LeaderElectionConfig
	if c.LeaderElectionConfiguration.LeaderElect {
		leaderElectionConfig, err = makeLeaderElectionConfig(c.LeaderElectionConfiguration, leaderElectionClient, leaderElectionRecorder)
		if err != nil {
			return nil, err
		}
	}

	c.Client = inClusterClient
	c.SecretClient = inClusterClient.CoreV1()
	c.VirtualClusterInformer = vcinformers.NewSharedInformerFactory(virtualClusterClient, 0).Tenancy().V1alpha1().Virtualclusters()
	c.SuperMasterClient = superMasterClient
	c.SuperMasterInformerFactory = informers.NewSharedInformerFactory(superMasterClient, 0)
	c.Broadcaster = eventBroadcaster
	c.Recorder = recorder
	c.LeaderElectionClient = leaderElectionClient
	c.LeaderElection = leaderElectionConfig

	return c, nil
}

// makeLeaderElectionConfig builds a leader election configuration. It will
// create a new resource lock associated with the configuration.
func makeLeaderElectionConfig(config componentbaseconfig.LeaderElectionConfiguration, client clientset.Interface, recorder record.EventRecorder) (*leaderelection.LeaderElectionConfig, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("unable to get hostname: %v", err)
	}
	// add a uniquifier so that two processes on the same host don't accidentally both become active
	id := hostname + "_" + string(uuid.NewUUID())

	rl, err := resourcelock.New(config.ResourceLock,
		config.ResourceNamespace,
		config.ResourceName,
		client.CoreV1(),
		client.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: recorder,
		})
	if err != nil {
		return nil, fmt.Errorf("couldn't create resource lock: %v", err)
	}

	return &leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: config.LeaseDuration.Duration,
		RenewDeadline: config.RenewDeadline.Duration,
		RetryPeriod:   config.RetryPeriod.Duration,
		WatchDog:      leaderelection.NewLeaderHealthzAdaptor(time.Second * 20),
		Name:          ResourceSyncerUserAgent,
	}, nil
}

// createClients creates a meta cluster kube client and a super master custer client from the given config and masterOverride.
func createClients(config componentbaseconfig.ClientConnectionConfiguration, masterOverride string, timeout time.Duration) (clientset.Interface,
	clientset.Interface, vcclient.Interface, clientset.Interface, error) {
	if len(config.Kubeconfig) == 0 && len(masterOverride) == 0 {
		klog.Warningf("Neither --kubeconfig nor --master was specified. Using default API client. This might not work.")
	}

	// This creates a client, first loading any specified kubeconfig
	// file, and then overriding the Master flag, if non-empty.
	superMasterKubeConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: config.Kubeconfig},
		&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterOverride}}).ClientConfig()
	if err != nil {
		return nil, nil, nil, nil, err
	}

	superMasterKubeConfig.ContentConfig.ContentType = config.AcceptContentTypes
	superMasterKubeConfig.QPS = config.QPS
	superMasterKubeConfig.Burst = int(config.Burst)

	superMasterClient, err := clientset.NewForConfig(restclient.AddUserAgent(superMasterKubeConfig, ResourceSyncerUserAgent))
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// creates the in-cluster config
	inClusterConfig, err := restclient.InClusterConfig()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	// creates the clientset
	client, err := kubernetes.NewForConfig(inClusterConfig)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	virtualClusterClient, err := vcclient.NewForConfig(inClusterConfig)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// shallow copy, do not modify the kubeConfig.Timeout.
	restConfig := *inClusterConfig
	restConfig.Timeout = timeout
	leaderElectionClient, err := clientset.NewForConfig(restclient.AddUserAgent(&restConfig, "leader-election"))
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return client, leaderElectionClient, virtualClusterClient, superMasterClient, nil
}
