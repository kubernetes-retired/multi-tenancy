package blockuseofhostnetworkingandports

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"log"
	"os"
	"strings"
	"testing"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/rbac/v1"
	apiextensionspkg "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	podutil "k8s.io/kubernetes/test/e2e/framework/pod"
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

	defer func() {
		testClient.DeletePolicy()
		testClient.DeleteRole()
	}()

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
	}{
		{
			testName:     "TestPreRunWithoutRole",
			testFunction: testPreRunWithoutRole,
			preRun:       false,
			run:          false,
		},
		{
			testName:     "TestPreRunWithRole",
			testFunction: testPreRunWithRole,
			preRun:       true,
			run:          false,
		},
		{
			testName:     "TestRunWithPolicy",
			testFunction: testRunWithPolicy,
			preRun:       true,
			run:          true,
		},
	}

	for _, tc := range tests {
		fmt.Println("Running test: ", tc.testName)
		preRun, run := tc.testFunction(t)
		g.Expect(preRun).To(gomega.Equal(tc.preRun))
		g.Expect(run).To(gomega.Equal(tc.run))
	}
}

func testPreRunWithoutRole(t *testing.T) (preRun bool, run bool) {
	err := b.PreRun(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		return false, false
	}
	return true, false
}

func testPreRunWithRole(t *testing.T) (preRun bool, run bool) {
	policies := []v1.PolicyRule{
		{
			Verbs:           []string{"get", "list", "watch", "create", "update", "patch", "delete"},
			APIGroups:       []string{""},
			Resources:       []string{"pods"},
			ResourceNames:   nil,
			NonResourceURLs: nil,
		},
	}

	createdRole, err := testClient.CreateRole("pod-role-6", policies)
	if err != nil {
		fmt.Println(err.Error())
	}

	_, err = testClient.CreateRoleBinding("pod-role-binding-6", createdRole)
	if err != nil {
		fmt.Println(err.Error())
	}

	err = b.PreRun(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		t.Logf(err.Error())
		return false, false
	}
	if err = b.Run(testClient.Namespace, testClient.K8sClient, tenantClient); err != nil {
		return true, false
	}
	return true, true
}

func testRunWithPolicy(t *testing.T) (preRun bool, run bool) {
	paths := []string{unittestutils.DisallowNetworkPorts}

	for _, p := range paths {
		err := testClient.CreatePolicy(p)
		if err != nil {
			fmt.Println(err.Error())
			return false, false
		}
	}

	podsList, err := testClient.Kubernetes.CoreV1().Pods("kyverno").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
	}
	podNames := []string{podsList.Items[0].ObjectMeta.Name}
	for {
		if podutil.CheckPodsRunningReady(testClient.Kubernetes, "kyverno", podNames, 200*time.Second) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	err = b.PreRun(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		fmt.Println(err.Error())
		return false, false
	}
	if err = b.Run(testClient.Namespace, testClient.K8sClient, tenantClient); err != nil {
		fmt.Println(err.Error())
		return true, false
	}
	return true, true
}
