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

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	clientgokubescheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	cliflag "k8s.io/component-base/cli/flag"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog"

	syncerappconfig "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/cmd/syncer/app/config"
	vcclient "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions"
	syncerconfig "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
)

const (
	// ResourceSyncerUserAgent is the userAgent name when starting resource syncer.
	ResourceSyncerUserAgent = "resource-syncer"
)

// ResourceSyncerOptions is the main context object for the resource syncer.
type ResourceSyncerOptions struct {
	// The syncer configuration.
	ComponentConfig syncerconfig.SyncerConfiguration

	SuperMaster           string
	SuperMasterKubeconfig string
}

// NewResourceSyncerOptions creates a new resource syncer with a default config.
func NewResourceSyncerOptions() (*ResourceSyncerOptions, error) {
	return &ResourceSyncerOptions{
		ComponentConfig: syncerconfig.SyncerConfiguration{
			LeaderElection:   syncerconfig.SyncerLeaderElectionConfiguration{},
			ClientConnection: componentbaseconfig.ClientConnectionConfiguration{},
		},
	}, nil
}

func (o *ResourceSyncerOptions) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}

	fs := fss.FlagSet("misc")
	fs.StringVar(&o.SuperMaster, "super-master", o.SuperMaster, "The address of the super master Kubernetes API server (overrides any value in super-master-kubeconfig).")
	fs.StringVar(&o.ComponentConfig.ClientConnection.Kubeconfig, "super-master-kubeconfig", o.ComponentConfig.ClientConnection.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")

	BindFlags(&o.ComponentConfig.LeaderElection, fss.FlagSet("leader election"))

	return fss
}

// BindFlags binds the LeaderElectionConfiguration struct fields to a flagset
func BindFlags(l *syncerconfig.SyncerLeaderElectionConfiguration, fs *pflag.FlagSet) {
	fs.BoolVar(&l.LeaderElect, "leader-elect", l.LeaderElect, ""+
		"Start a leader election client and gain leadership before "+
		"executing the main loop. Enable this when running replicated "+
		"components for high availability.")
	fs.DurationVar(&l.LeaseDuration.Duration, "leader-elect-lease-duration", l.LeaseDuration.Duration, ""+
		"The duration that non-leader candidates will wait after observing a leadership "+
		"renewal until attempting to acquire leadership of a led but unrenewed leader "+
		"slot. This is effectively the maximum duration that a leader can be stopped "+
		"before it is replaced by another candidate. This is only applicable if leader "+
		"election is enabled.")
	fs.DurationVar(&l.RenewDeadline.Duration, "leader-elect-renew-deadline", l.RenewDeadline.Duration, ""+
		"The interval between attempts by the acting master to renew a leadership slot "+
		"before it stops leading. This must be less than or equal to the lease duration. "+
		"This is only applicable if leader election is enabled.")
	fs.DurationVar(&l.RetryPeriod.Duration, "leader-elect-retry-period", l.RetryPeriod.Duration, ""+
		"The duration the clients should wait between attempting acquisition and renewal "+
		"of a leadership. This is only applicable if leader election is enabled.")
	fs.StringVar(&l.ResourceLock, "leader-elect-resource-lock", l.ResourceLock, ""+
		"The type of resource object that is used for locking during "+
		"leader election. Supported options are `endpoints` (default) and `configmaps`.")
	fs.StringVar(&l.LockObjectNamespace, "lock-object-namespace", l.LockObjectNamespace, "DEPRECATED: define the namespace of the lock object.")
	fs.StringVar(&l.LockObjectName, "lock-object-name", l.LockObjectName, "DEPRECATED: define the name of the lock object.")

}

// Config return a syncer config object
func (o *ResourceSyncerOptions) Config() (*syncerappconfig.Config, error) {
	c := &syncerappconfig.Config{}
	c.ComponentConfig = o.ComponentConfig

	// Prepare kube clients
	leaderElectionClient, virtualClusterClient, superMasterClient, err := createClients(c.ComponentConfig.ClientConnection, o.SuperMaster, c.ComponentConfig.LeaderElection.RenewDeadline.Duration)
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
	if c.ComponentConfig.LeaderElection.LeaderElect {
		leaderElectionConfig, err = makeLeaderElectionConfig(c.ComponentConfig.LeaderElection, leaderElectionClient, leaderElectionRecorder)
		if err != nil {
			return nil, err
		}
	}

	c.SecretClient = superMasterClient.CoreV1()
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
func makeLeaderElectionConfig(config syncerconfig.SyncerLeaderElectionConfiguration, client clientset.Interface, recorder record.EventRecorder) (*leaderelection.LeaderElectionConfig, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("unable to get hostname: %v", err)
	}
	// add a uniquifier so that two processes on the same host don't accidentally both become active
	id := hostname + "_" + string(uuid.NewUUID())

	rl, err := resourcelock.New(config.ResourceLock,
		config.LockObjectNamespace,
		config.LockObjectName,
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
	vcclient.Interface, clientset.Interface, error) {
	if len(config.Kubeconfig) == 0 && len(masterOverride) == 0 {
		klog.Warningf("Neither --kubeconfig nor --master was specified. Using in-cluster API client.")
	}

	// This creates a client, first loading any specified kubeconfig
	// file, and then overriding the Master flag, if non-empty.
	var restConfig *restclient.Config
	var err error
	if len(config.Kubeconfig) == 0 {
		restConfig, err = restclient.InClusterConfig()
	} else {
		var overrideConfig *clientcmd.ConfigOverrides
		if len(masterOverride) != 0 {
			overrideConfig = &clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterOverride}}
		}

		restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: config.Kubeconfig}, overrideConfig).ClientConfig()
	}

	if err != nil {
		return nil, nil, nil, err
	}

	restConfig.ContentConfig.ContentType = config.AcceptContentTypes
	restConfig.QPS = config.QPS
	restConfig.Burst = int(config.Burst)

	superMasterClient, err := clientset.NewForConfig(restclient.AddUserAgent(restConfig, ResourceSyncerUserAgent))
	if err != nil {
		return nil, nil, nil, err
	}

	virtualClusterClient, err := vcclient.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	// shallow copy, do not modify the kubeConfig.Timeout.
	leaderElectionRestConfig := *restConfig
	restConfig.Timeout = timeout
	leaderElectionClient, err := clientset.NewForConfig(restclient.AddUserAgent(&leaderElectionRestConfig, "leader-election"))
	if err != nil {
		return nil, nil, nil, err
	}

	return leaderElectionClient, virtualClusterClient, superMasterClient, nil
}
