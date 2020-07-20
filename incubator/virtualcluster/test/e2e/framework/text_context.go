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

package framework

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/onsi/ginkgo/config"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog"
)

const (
	defaultHost = "http://127.0.0.1:8080"
)

// TestContextType contains test settings and global state.
type TestContextType struct {
	KubeConfig  string
	KubeContext string
	Host        string

	ReportDir    string
	ReportPrefix string

	DeleteNamespace          bool
	DeleteNamespaceOnFailure bool

	// If set to true test will dump data about the namespace in which test was running.
	DumpLogsOnFailure bool
}

// TestContext should be used by all tests to access common context data.
var TestContext TestContextType

// RegisterCommonFlags registers flags common to all e2e test suites.
// The flag set can be flag.CommandLine (if desired) or a custom
// flag set that then gets passed to viperconfig.ViperizeFlags.
//
// The other Register*Flags methods below can be used to add more
// test-specific flags. However, those settings then get added
// regardless whether the test is actually in the test suite.
func RegisterCommonFlags(flags *flag.FlagSet) {
	// Turn on verbose by default to get spec names
	config.DefaultReporterConfig.Verbose = true

	// Turn on EmitSpecProgress to get spec progress (especially on interrupt)
	config.GinkgoConfig.EmitSpecProgress = true

	// Randomize specs as well as suites
	config.GinkgoConfig.RandomizeAllSpecs = true

	flags.BoolVar(&TestContext.DumpLogsOnFailure, "dump-logs-on-failure", true, "If set to true test will dump data about the namespace in which test was running.")
	flags.BoolVar(&TestContext.DeleteNamespace, "delete-namespace", true, "If true tests will delete namespace after completion. It is only designed to make debugging easier, DO NOT turn it off by default.")
	flags.BoolVar(&TestContext.DeleteNamespaceOnFailure, "delete-namespace-on-failure", true, "If true, framework will delete test namespace on failure. Used only during test debugging.")

	flags.StringVar(&TestContext.Host, "host", "", fmt.Sprintf("The host, or apiserver, to connect to. Will default to %s if this argument and --kubeconfig are not set", defaultHost))
	flags.StringVar(&TestContext.ReportPrefix, "report-prefix", "", "Optional prefix for JUnit XML reports. Default is empty, which doesn't prepend anything to the default name.")
	flags.StringVar(&TestContext.ReportDir, "report-dir", "", "Path to the directory where the JUnit XML reports should be saved. Default is empty, which doesn't generate these reports.")
}

// RegisterClusterFlags registers flags specific to the cluster e2e test suite.
func RegisterClusterFlags(flags *flag.FlagSet) {
	flags.StringVar(&TestContext.KubeConfig, clientcmd.RecommendedConfigPathFlag, os.Getenv(clientcmd.RecommendedConfigPathEnvVar), "Path to kubeconfig containing embedded authinfo.")
	flags.StringVar(&TestContext.KubeContext, clientcmd.FlagContext, "", "kubeconfig context to use/override. If unset, will use value from 'current-context'")
}

// HandleFlags sets up all flags and parses the command line.
func HandleFlags() {
	RegisterCommonFlags(flag.CommandLine)
	RegisterClusterFlags(flag.CommandLine)
	flag.Parse()
}

func createKubeConfig(clientCfg *restclient.Config) *clientcmdapi.Config {
	clusterNick := "cluster"
	userNick := "user"
	contextNick := "context"

	config := clientcmdapi.NewConfig()

	credentials := clientcmdapi.NewAuthInfo()
	credentials.Token = clientCfg.BearerToken
	credentials.TokenFile = clientCfg.BearerTokenFile
	credentials.ClientCertificate = clientCfg.TLSClientConfig.CertFile
	if len(credentials.ClientCertificate) == 0 {
		credentials.ClientCertificateData = clientCfg.TLSClientConfig.CertData
	}
	credentials.ClientKey = clientCfg.TLSClientConfig.KeyFile
	if len(credentials.ClientKey) == 0 {
		credentials.ClientKeyData = clientCfg.TLSClientConfig.KeyData
	}
	config.AuthInfos[userNick] = credentials

	cluster := clientcmdapi.NewCluster()
	cluster.Server = clientCfg.Host
	cluster.CertificateAuthority = clientCfg.CAFile
	if len(cluster.CertificateAuthority) == 0 {
		cluster.CertificateAuthorityData = clientCfg.CAData
	}
	cluster.InsecureSkipTLSVerify = clientCfg.Insecure
	config.Clusters[clusterNick] = cluster

	context := clientcmdapi.NewContext()
	context.Cluster = clusterNick
	context.AuthInfo = userNick
	config.Contexts[contextNick] = context
	config.CurrentContext = contextNick

	return config
}

// AfterReadingAllFlags makes changes to the context after all flags
// have been read.
func AfterReadingAllFlags(t *TestContextType) {
	// Only set a default host if one won't be supplied via kubeconfig
	if len(t.Host) == 0 && len(t.KubeConfig) == 0 {
		// Check if we can use the in-cluster config
		if clusterConfig, err := restclient.InClusterConfig(); err == nil {
			if tempFile, err := ioutil.TempFile(os.TempDir(), "kubeconfig-"); err == nil {
				kubeConfig := createKubeConfig(clusterConfig)
				clientcmd.WriteToFile(*kubeConfig, tempFile.Name())
				t.KubeConfig = tempFile.Name()
				klog.Infof("Using a temporary kubeconfig file from in-cluster config : %s", tempFile.Name())
			}
		}
		if len(t.KubeConfig) == 0 {
			klog.Warningf("Unable to find in-cluster config, using default host : %s", defaultHost)
			t.Host = defaultHost
		}
	}
}
