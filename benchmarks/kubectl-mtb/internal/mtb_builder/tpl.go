package mtb_builder

// BenchmarkFileTemplate returns the main file template
func BenchmarkFileTemplate() []byte {
	return []byte(`package {{ .PkgName }}

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
)

var b = &benchmark.Benchmark{

	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		return nil
	},
	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("{{ .FileName }}/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b);
}
	`)
}

// BenchmarkTestTemplate returns benchmarks test file template
func BenchmarkTestTemplate() []byte {
	return []byte(`package {{ .PkgName }}
	
import (
	"fmt"
	"path/filepath"

	"log"
	"os"
	"strings"
	"testing"

	"github.com/onsi/gomega"
	apiextensionspkg "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/unittestutils"
)
	
var testClient *unittestutils.TestClient
var tenantConfig *rest.Config
var tenantClient *kubernetes.Clientset
var tenantNamespace = "tenant1admin"
var serviceAccount = "t1-admin1"
var g *gomega.GomegaWithT

type TestFunction func(t *testing.T) (bool, bool)

func TestMain(m *testing.M) {
	var clusterExists bool
	kind := &unittestutils.KindCluster{}
	setUp := func() error {
		provider := cluster.NewProvider()
		// List the clusters available
		clusterList, err := provider.List()
		clusters := strings.Join(clusterList, " ")
		// Checks if the main cluster (test) is running
		if !strings.Contains(clusters, "kubectl-mtb-suite") {
			err := kind.CreateCluster()
			clusterExists = false
			if err != nil {
				return err
			}
		} else {
			clusterExists = true
		}
		kubecfgFlags := genericclioptions.NewConfigFlags(false)

		// Create the K8s clientSet
		cfg, err := kubecfgFlags.ToRESTConfig()
		k8sClient, err := kubernetes.NewForConfig(cfg)

		if err != nil {
			return err
		}
		rest := k8sClient.CoreV1().RESTClient()
		var apiExtensions *apiextensionspkg.Clientset
		apiExtensions, err = apiextensionspkg.NewForConfig(cfg)
		// Initialize testclient
		testClient = unittestutils.TestNewClient("unittests", k8sClient, apiExtensions, rest, cfg)
		tenantConfig := testClient.Config
		tenantConfig.Impersonate.UserName = "system:serviceaccount:default:t1-admin1"
		tenantClient, _ = kubernetes.NewForConfig(tenantConfig)
		// Install Kyverno
		path := filepath.Join("..", "..", "assets")
		crdPath := filepath.Join(path, "kyverno.yaml")
		err = testClient.CreatePolicy(crdPath)
		if err != nil {
			fmt.Println(err.Error())
		}
		return err
	}
	//exec setUp function
	err := setUp()

	if err != nil {
		log.Print(err.Error())
		os.Exit(1)
	}

	// exec test and this returns an exit code to pass to os
	retCode := m.Run()

	tearDown := func() error {
		var err error
		if !clusterExists {
			err := kind.DeleteCluster()
			return err
		} else {
			unittestutils.DestroyTenant(g)
		}
		return err
	}

	// exec tearDown function
	err = tearDown()
	if err != nil {
		log.Print(err.Error())
	}

	os.Exit(retCode)
}

func testCreateTenants(t *testing.T, g *gomega.GomegaWithT, namespace string, serviceAcc string) {
	err := unittestutils.CreateCrds()
	fmt.Println("Creating tenants")
	unittestutils.CreateTenant(t, g, namespace, serviceAcc)
	if err != nil {
		t.Error(err.Error())
	}
}

func TestBenchmark(t *testing.T) {
	g = gomega.NewGomegaWithT(t)
	testClient.Namespace = tenantNamespace
	testClient.ServiceAccount = serviceAccount
	// test to create tenants
	testCreateTenants(t, g, tenantNamespace, serviceAccount)
	tests := []struct {
		testName     string
		testFunction TestFunction
		preRun       bool
		run          bool
	}{}

	for _, tc := range tests {
		fmt.Println("Running test: ", tc.testName)
		preRun, run := tc.testFunction(t)
		g.Expect(preRun).To(gomega.Equal(tc.preRun))
		g.Expect(run).To(gomega.Equal(tc.run))
	}

}		
`)
}

// ConfigYamlTemplate returns the config file template
func ConfigYamlTemplate() []byte {
	return []byte(
		`id: {{ .ID }}
title: {{ .Title }}
benchmarkType: {{ .BenchmarkType }}
category: {{ .Category }} 
description: {{ .Description }}
remediation: {{ .Remediation }}
profileLevel: {{ .ProfileLevel  }}`)
}
