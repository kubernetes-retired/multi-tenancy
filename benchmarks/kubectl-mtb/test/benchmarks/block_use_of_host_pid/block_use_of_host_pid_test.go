package blockuseofhostpid

import (
	"fmt"

	"os"
	"strings"
	"testing"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/rbac/v1"
	apiextensionspkg "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/unittestutils"
)

var (
	testClient *unittestutils.TestClient
	tenantConfig *rest.Config
	tenantClient *kubernetes.Clientset
	clusterExists bool
	saNamespace = "default"
	tenantName = "tenant1"
	tenantAdminNamespaceName = "tenant1admin"
	tenantNamespaceName = "tenantnamespace1"
	actualTenantNamespaceName = "t1-ns1"
	saName = "tenant1-admin"
	apiExtensions *apiextensionspkg.Clientset
	g *gomega.GomegaWithT
)

type TestFunction func(t *testing.T) (bool, bool)

func TestMain(m *testing.M) {
	// Create kind instance
	kind := &unittestutils.KindCluster{}

	// Tenant setup function
	setUp := func() error {
		provider := cluster.NewProvider()
		
		// List the clusters available
		clusterList, err := provider.List()
		clusters := strings.Join(clusterList, " ")

		// Checks if the main cluster (test) is running
		if strings.Contains(clusters, "kubectl-mtb-suite") {
			clusterExists = true
		} else {
			clusterExists = false
			err := kind.CreateCluster()
			if err != nil {
				return err
			}
		}

		kubecfgFlags := genericclioptions.NewConfigFlags(false)

		// Create the K8s clientSet
		cfg, err := kubecfgFlags.ToRESTConfig()
		k8sClient, err := kubernetes.NewForConfig(cfg)
		if err != nil {
			return err
		}
		rest := k8sClient.CoreV1().RESTClient()
		apiExtensions, err = apiextensionspkg.NewForConfig(cfg)
		
		// Initialize testclient
		testClient = unittestutils.TestNewClient("unittests", k8sClient, apiExtensions, rest, cfg)
		tenantConfig := testClient.Config
		tenantConfig.Impersonate.UserName = "system:serviceaccount:" + saNamespace  + ":" + saName
		tenantClient, _ = kubernetes.NewForConfig(tenantConfig)
		testClient.Namespace = actualTenantNamespaceName
		testClient.ServiceAccount = unittestutils.ServiceAccountObj(saName, saNamespace)
		return nil
	}

	//exec setUp function
	err := setUp()

	if err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
		os.Exit(1)
	}

	// exec test and this returns an exit code to pass to os
	retCode := m.Run()

	tearDown := func() error {
		var err error
		if !clusterExists {
			err := kind.DeleteCluster()
			if err != nil {
				return err
			}
		} 
		return err
	}

	// exec tearDown function
	err = tearDown()
	if err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	os.Exit(retCode)
}

func CreateTenants(t *testing.T, g *gomega.GomegaWithT) {
	err := unittestutils.CreateCrds(testClient)
	if err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	unittestutils.ServiceAccounts = append(unittestutils.ServiceAccounts, unittestutils.ServiceAccountObj(saName, saNamespace))
	unittestutils.Tenants = append(unittestutils.Tenants, unittestutils.TenantObj(tenantName,  unittestutils.ServiceAccountObj(saName, saNamespace), tenantAdminNamespaceName))
	unittestutils.Tenantnamespaces = append(unittestutils.Tenantnamespaces, unittestutils.TenantNamespaceObj(tenantNamespaceName, tenantAdminNamespaceName, actualTenantNamespaceName))

	fmt.Println("Creating tenants")
	unittestutils.CreateTenant(t, g)
}

func TestBenchmark(t *testing.T) {
	defer func() {
		testClient.DeletePolicy()
		testClient.DeleteRole()
	}()

	g = gomega.NewGomegaWithT(t)
	// test to create tenants

	if !clusterExists {
		CreateTenants(t, g)
	}

	if !unittestutils.CheckNamespaceExist(actualTenantNamespaceName, testClient.K8sClient) {
		CreateTenants(t, g)
	}

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

	createdRole, err := testClient.CreateRole("pod-role", policies)
	if err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	_, err = testClient.CreateRoleBinding("pod-role-binding", createdRole)
	if err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	err = b.PreRun(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		return false, false
	}
	if err = b.Run(testClient.Namespace, testClient.K8sClient, tenantClient); err != nil {
		return true, false
	}
	return true, true
}

func testRunWithPolicy(t *testing.T) (preRun bool, run bool) {
	paths := []string{unittestutils.DisallowHostPID}

	for _, p := range paths {
		err := testClient.CreatePolicy(p)
		if err != nil {
			return false, false
		}

		unittestutils.WaitForPolicy()
	}


	err := b.PreRun(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		fmt.Println(err.Error())
		return false, false
	}

	err = b.Run(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		fmt.Println(err.Error())
		return true, false
	}

	return true, true
}
