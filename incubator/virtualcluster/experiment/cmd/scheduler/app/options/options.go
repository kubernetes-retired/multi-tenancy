/*
Copyright 2020 The Kubernetes Authors.

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
	"io/ioutil"
	"os"
	"time"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	clientgokubescheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	cliflag "k8s.io/component-base/cli/flag"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis"

	schedulerappconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/cmd/scheduler/app/config"
	superclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/clientset/versioned"
	superinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/client/informers/externalversions"
	schedulerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/experiment/pkg/scheduler/constants"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions"
)

type SchedulerOptions struct {
	// The syncer configuration.
	ComponentConfig schedulerconfig.SchedulerConfiguration

	MetaCluster           string
	MetaClusterKubeconfig string
}

// NewSchedulerOptions creates new scheduler options with a default config.
func NewSchedulerOptions() (*SchedulerOptions, error) {
	return &SchedulerOptions{
		ComponentConfig: schedulerconfig.SchedulerConfiguration{
			LeaderElection: schedulerconfig.SchedulerLeaderElectionConfiguration{
				LeaderElectionConfiguration: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:   true,
					LeaseDuration: v1.Duration{Duration: 15 * time.Second},
					RenewDeadline: v1.Duration{Duration: 10 * time.Second},
					RetryPeriod:   v1.Duration{Duration: 2 * time.Second},
					ResourceLock:  resourcelock.ConfigMapsResourceLock,
				},
				LockObjectName: "vc-scheduler-leaderelection-lock",
			},
			ClientConnection: componentbaseconfig.ClientConnectionConfiguration{},
		},
	}, nil
}

func (o *SchedulerOptions) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}

	fs := fss.FlagSet("server")
	fs.StringVar(&o.MetaCluster, "meta-cluster", o.MetaCluster, "The address of the meta cluster Kubernetes APIServer (overrides any value in meta-cluster-kubeconfig).")
	fs.StringVar(&o.ComponentConfig.ClientConnection.Kubeconfig, "meta-master-kubeconfig", o.ComponentConfig.ClientConnection.Kubeconfig, "Path to kubeconfig file with authorization and meta cluster location information.")

	BindFlags(&o.ComponentConfig.LeaderElection, fss.FlagSet("leader election"))

	return fss
}

// BindFlags binds the LeaderElectionConfiguration struct fields to a flagset
func BindFlags(l *schedulerconfig.SchedulerLeaderElectionConfiguration, fs *pflag.FlagSet) {
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
		"leader election. Supported options are `endpoints` and `configmaps` (default).")
	fs.StringVar(&l.LockObjectNamespace, "lock-object-namespace", l.LockObjectNamespace, "DEPRECATED: define the namespace of the lock object.")
	fs.StringVar(&l.LockObjectName, "lock-object-name", l.LockObjectName, "DEPRECATED: define the name of the lock object.")
}

// Config return a syncer config object
func (o *SchedulerOptions) Config() (*schedulerappconfig.Config, error) {
	c := &schedulerappconfig.Config{}
	c.ComponentConfig = o.ComponentConfig

	// Prepare kube clients
	leaderElectionClient, metaClusterClient, virtualClusterClient, superClusterClient, restConfig, err := createClients(c.ComponentConfig.ClientConnection, o.MetaCluster, c.ComponentConfig.LeaderElection.RenewDeadline.Duration)
	if err != nil {
		return nil, err
	}

	// Prepare event clients.
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(clientgokubescheme.Scheme, corev1.EventSource{Component: constants.SchedulerUserAgent})
	leaderElectionBroadcaster := record.NewBroadcaster()
	leaderElectionRecorder := leaderElectionBroadcaster.NewRecorder(clientgokubescheme.Scheme, corev1.EventSource{Component: constants.SchedulerUserAgent})

	// Set up leader election if enabled.
	var leaderElectionConfig *leaderelection.LeaderElectionConfig
	if c.ComponentConfig.LeaderElection.LeaderElect {
		leaderElectionConfig, err = makeLeaderElectionConfig(c.ComponentConfig.LeaderElection, leaderElectionClient, leaderElectionRecorder)
		if err != nil {
			return nil, err
		}
	}

	// Setup Scheme for all resources
	if err := apis.AddToScheme(clientgokubescheme.Scheme); err != nil {
		return nil, err
	}

	c.ComponentConfig.RestConfig = restConfig
	c.VirtualClusterClient = virtualClusterClient
	c.VirtualClusterInformer = vcinformers.NewSharedInformerFactory(virtualClusterClient, 0).Tenancy().V1alpha1().VirtualClusters()
	c.SuperClusterClient = superClusterClient
	c.SuperClusterInformer = superinformers.NewSharedInformerFactory(superClusterClient, 0).Cluster().V1alpha4().Clusters()
	c.MetaClusterClient = metaClusterClient
	c.MetaClusterInformerFactory = informers.NewSharedInformerFactory(metaClusterClient, 0)
	c.Broadcaster = eventBroadcaster
	c.Recorder = recorder
	c.LeaderElectionClient = leaderElectionClient
	c.LeaderElection = leaderElectionConfig

	return c, nil
}

// makeLeaderElectionConfig builds a leader election configuration. It will
// create a new resource lock associated with the configuration.
func makeLeaderElectionConfig(config schedulerconfig.SchedulerLeaderElectionConfiguration, client clientset.Interface, recorder record.EventRecorder) (*leaderelection.LeaderElectionConfig, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("unable to get hostname: %v", err)
	}
	// add a uniquifier so that two processes on the same host don't accidentally both become active
	id := hostname + "_" + string(uuid.NewUUID())

	if config.LockObjectNamespace == "" {
		var err error
		config.LockObjectNamespace, err = getInClusterNamespace()
		if err != nil {
			return nil, fmt.Errorf("unable to find leader election namespace: %v", err)
		}
	}

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
		Name:          constants.SchedulerUserAgent,
	}, nil
}

func getInClusterNamespace() (string, error) {
	// Check whether the namespace file exists.
	// If not, we are not running in cluster so can't guess the namespace.
	_, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if os.IsNotExist(err) {
		return "", fmt.Errorf("not running in-cluster, please specify LeaderElectionNamespace")
	} else if err != nil {
		return "", fmt.Errorf("error checking namespace file: %v", err)
	}

	// Load the namespace file and return its content
	namespace, err := ioutil.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "", fmt.Errorf("error reading namespace file: %v", err)
	}
	return string(namespace), nil
}

func createClients(config componentbaseconfig.ClientConnectionConfiguration, masterOverride string, timeout time.Duration) (clientset.Interface,
	clientset.Interface, vcclient.Interface, superclient.Interface, *restclient.Config, error) {
	// This creates a client, first loading any specified kubeconfig
	// file, and then overriding the Master flag, if non-empty.
	var (
		restConfig *restclient.Config
		err        error
	)
	if len(config.Kubeconfig) == 0 && len(masterOverride) == 0 {
		klog.Info("Neither kubeconfig file nor master URL was specified. Falling back to in-cluster config.")
		restConfig, err = restclient.InClusterConfig()
	} else {
		// This creates a client, first loading any specified kubeconfig
		// file, and then overriding the Master flag, if non-empty.
		restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: config.Kubeconfig},
			&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterOverride}}).ClientConfig()
	}

	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	if restConfig.Timeout == 0 {
		restConfig.Timeout = constants.DefaultRequestTimeout
	}

	restConfig.ContentConfig.ContentType = config.AcceptContentTypes
	restConfig.QPS = config.QPS
	if restConfig.QPS == 0 {
		restConfig.QPS = constants.DefaultSchedulerClientQPS
	}
	restConfig.Burst = int(config.Burst)
	if restConfig.Burst == 0 {
		restConfig.Burst = constants.DefaultSchedulerClientBurst
	}

	metaClusterClient, err := clientset.NewForConfig(restclient.AddUserAgent(restConfig, constants.SchedulerUserAgent))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	virtualClusterClient, err := vcclient.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	superClusterClient, err := superclient.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	leaderElectionRestConfig := *restConfig
	restConfig.Timeout = timeout
	leaderElectionClient, err := clientset.NewForConfig(restclient.AddUserAgent(&leaderElectionRestConfig, "leader-election"))
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}

	return leaderElectionClient, metaClusterClient, virtualClusterClient, superClusterClient, restConfig, nil
}
