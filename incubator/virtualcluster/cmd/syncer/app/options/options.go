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
	"k8s.io/client-go/kubernetes/scheme"
	clientgokubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	cliflag "k8s.io/component-base/cli/flag"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog"

	syncerappconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/cmd/syncer/app/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/apis"
	vcclient "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/clientset/versioned"
	vcinformers "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/client/informers/externalversions"
	syncerconfig "sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/apis/config"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/syncer/util/featuregate"
	"sigs.k8s.io/multi-tenancy/incubator/virtualcluster/pkg/util/constants"
)

// ResourceSyncerOptions is the main context object for the resource syncer.
type ResourceSyncerOptions struct {
	// The syncer configuration.
	ComponentConfig syncerconfig.SyncerConfiguration

	SuperMaster           string
	SuperMasterKubeconfig string
	SyncerName            string
	Address               string
	Port                  string
	CertFile              string
	KeyFile               string
}

// NewResourceSyncerOptions creates a new resource syncer with a default config.
func NewResourceSyncerOptions() (*ResourceSyncerOptions, error) {
	return &ResourceSyncerOptions{
		ComponentConfig: syncerconfig.SyncerConfiguration{
			LeaderElection: syncerconfig.SyncerLeaderElectionConfiguration{
				LeaderElectionConfiguration: componentbaseconfig.LeaderElectionConfiguration{
					LeaderElect:   true,
					LeaseDuration: v1.Duration{Duration: 15 * time.Second},
					RenewDeadline: v1.Duration{Duration: 10 * time.Second},
					RetryPeriod:   v1.Duration{Duration: 2 * time.Second},
					ResourceLock:  resourcelock.ConfigMapsResourceLock,
				},
				LockObjectName: "syncer-leaderelection-lock",
			},
			ClientConnection:           componentbaseconfig.ClientConnectionConfiguration{},
			DisableServiceAccountToken: true,
			DefaultOpaqueMetaDomains:   []string{"kubernetes.io", "k8s.io"},
			ExtraSyncingResources:      []string{},
			VNAgentPort:                int32(10550),
			FeatureGates: map[string]bool{
				featuregate.SuperClusterPooling:        false,
				featuregate.SuperClusterServiceNetwork: false,
			},
		},
		SyncerName: "vc",
		Address:    "",
		Port:       "80",
		CertFile:   "",
		KeyFile:    "",
	}, nil
}

func (o *ResourceSyncerOptions) Flags() cliflag.NamedFlagSets {
	fss := cliflag.NamedFlagSets{}

	fs := fss.FlagSet("server")
	fs.StringVar(&o.SuperMaster, "super-master", o.SuperMaster, "The address of the super master Kubernetes API server (overrides any value in super-master-kubeconfig).")
	fs.StringVar(&o.ComponentConfig.ClientConnection.Kubeconfig, "super-master-kubeconfig", o.ComponentConfig.ClientConnection.Kubeconfig, "Path to kubeconfig file with authorization and master location information.")
	fs.StringVar(&o.SyncerName, "syncer-name", o.SyncerName, "Syncer name (default vc).")
	fs.BoolVar(&o.ComponentConfig.DisableServiceAccountToken, "disable-service-account-token", o.ComponentConfig.DisableServiceAccountToken, "DisableServiceAccountToken indicates whether disable service account token automatically mounted.")
	fs.StringSliceVar(&o.ComponentConfig.DefaultOpaqueMetaDomains, "default-opaque-meta-domains", o.ComponentConfig.DefaultOpaqueMetaDomains, "DefaultOpaqueMetaDomains is the default opaque meta configuration for each Virtual Cluster.")
	fs.StringSliceVar(&o.ComponentConfig.ExtraSyncingResources, "extra-syncing-resources", o.ComponentConfig.ExtraSyncingResources, "ExtraSyncingResources defines additional resources that need to be synced for each Virtual Cluster. (priorityclass, ingress)")
	fs.Int32Var(&o.ComponentConfig.VNAgentPort, "vn-agent-port", 10550, "Port the vn-agent listens on")
	fs.Var(cliflag.NewMapStringBool(&o.ComponentConfig.FeatureGates), "feature-gates", "A set of key=value pairs that describe featuregate gates for various features. ")

	serverFlags := fss.FlagSet("metricsServer")
	serverFlags.StringVar(&o.Address, "address", o.Address, "The server address.")
	serverFlags.StringVar(&o.Port, "port", o.Port, "The server port.")
	serverFlags.StringVar(&o.CertFile, "cert-file", o.CertFile, "CertFile is the file containing x509 Certificate for HTTPS.")
	serverFlags.StringVar(&o.KeyFile, "key-file", o.KeyFile, "KeyFile is the file containing x509 private key matching certFile.")

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
		"leader election. Supported options are `endpoints` and `configmaps` (default).")
	fs.StringVar(&l.LockObjectNamespace, "lock-object-namespace", l.LockObjectNamespace, "DEPRECATED: define the namespace of the lock object.")
	fs.StringVar(&l.LockObjectName, "lock-object-name", l.LockObjectName, "DEPRECATED: define the name of the lock object.")
}

// Config return a syncer config object
func (o *ResourceSyncerOptions) Config() (*syncerappconfig.Config, error) {
	c := &syncerappconfig.Config{}
	c.ComponentConfig = o.ComponentConfig

	// Prepare kube clients
	leaderElectionClient, virtualClusterClient, superMasterClient, restConfig, err := createClients(c.ComponentConfig.ClientConnection, o.SuperMaster, c.ComponentConfig.LeaderElection.RenewDeadline.Duration)
	if err != nil {
		return nil, err
	}

	// Prepare event clients.
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(clientgokubescheme.Scheme, corev1.EventSource{Component: constants.ResourceSyncerUserAgent})
	leaderElectionBroadcaster := record.NewBroadcaster()
	leaderElectionRecorder := leaderElectionBroadcaster.NewRecorder(clientgokubescheme.Scheme, corev1.EventSource{Component: constants.ResourceSyncerUserAgent})

	// Set up leader election if enabled.
	var leaderElectionConfig *leaderelection.LeaderElectionConfig
	if c.ComponentConfig.LeaderElection.LeaderElect {
		leaderElectionConfig, err = makeLeaderElectionConfig(c.ComponentConfig.LeaderElection, leaderElectionClient, leaderElectionRecorder, o.SyncerName)
		if err != nil {
			return nil, err
		}
	}

	featuregate.DefaultFeatureGate, err = featuregate.NewFeatureGate(c.ComponentConfig.FeatureGates)
	if err != nil {
		return nil, err
	}

	// Setup Scheme for all resources
	if err := apis.AddToScheme(scheme.Scheme); err != nil {
		return nil, err
	}
	c.ComponentConfig.RestConfig = restConfig
	c.SuperClient = superMasterClient.CoreV1()
	c.VirtualClusterClient = virtualClusterClient
	c.VirtualClusterInformer = vcinformers.NewSharedInformerFactory(virtualClusterClient, 0).Tenancy().V1alpha1().VirtualClusters()
	c.SuperMasterClient = superMasterClient
	c.SuperMasterInformerFactory = informers.NewSharedInformerFactory(superMasterClient, 0)
	c.Broadcaster = eventBroadcaster
	c.Recorder = recorder
	c.LeaderElectionClient = leaderElectionClient
	c.LeaderElection = leaderElectionConfig

	c.Address = o.Address
	c.Port = o.Port
	c.CertFile = o.CertFile
	c.KeyFile = o.KeyFile

	return c, nil
}

// makeLeaderElectionConfig builds a leader election configuration. It will
// create a new resource lock associated with the configuration.
func makeLeaderElectionConfig(config syncerconfig.SyncerLeaderElectionConfiguration, client clientset.Interface, recorder record.EventRecorder, syncername string) (*leaderelection.LeaderElectionConfig, error) {
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
	config.LockObjectName = syncername + "-" + "syncer-leaderelection-lock"
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
		Name:          constants.ResourceSyncerUserAgent,
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

// createClients creates a meta cluster kube client and a super master custer client from the given config and masterOverride.
func createClients(config componentbaseconfig.ClientConnectionConfiguration, masterOverride string, timeout time.Duration) (clientset.Interface,
	vcclient.Interface, clientset.Interface, *restclient.Config, error) {
	// This creates a client, first loading any specified kubeconfig
	// file, and then overriding the Master flag, if non-empty.
	var (
		restConfig *restclient.Config
		err        error
	)
	if len(config.Kubeconfig) == 0 && len(masterOverride) == 0 {
		klog.Info("Neither kubeconfig file nor master URL was specified. Falling back to in-cluster config.")
		restConfig, err = rest.InClusterConfig()
	} else {
		// This creates a client, first loading any specified kubeconfig
		// file, and then overriding the Master flag, if non-empty.
		restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			&clientcmd.ClientConfigLoadingRules{ExplicitPath: config.Kubeconfig},
			&clientcmd.ConfigOverrides{ClusterInfo: clientcmdapi.Cluster{Server: masterOverride}}).ClientConfig()
	}

	if err != nil {
		return nil, nil, nil, nil, err
	}

	if restConfig.Timeout == 0 {
		restConfig.Timeout = constants.DefaultRequestTimeout
	}

	restConfig.ContentConfig.ContentType = config.AcceptContentTypes
	restConfig.QPS = config.QPS
	if restConfig.QPS == 0 {
		restConfig.QPS = constants.DefaultSyncerClientQPS
	}
	restConfig.Burst = int(config.Burst)
	if restConfig.Burst == 0 {
		restConfig.Burst = constants.DefaultSyncerClientBurst
	}

	superMasterClient, err := clientset.NewForConfig(restclient.AddUserAgent(restConfig, constants.ResourceSyncerUserAgent))
	if err != nil {
		return nil, nil, nil, nil, err
	}

	virtualClusterClient, err := vcclient.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// shallow copy, do not modify the kubeConfig.Timeout.
	leaderElectionRestConfig := *restConfig
	restConfig.Timeout = timeout
	leaderElectionClient, err := clientset.NewForConfig(restclient.AddUserAgent(&leaderElectionRestConfig, "leader-election"))
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return leaderElectionClient, virtualClusterClient, superMasterClient, restConfig, nil
}
