package blockprivilegedcontainers

import (
	"fmt"
	apiextensionspkg "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"path/filepath"

	//apiextensionspkg "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"log"
	"os"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/unittestutils"
	"testing"
)

var testClient *unittestutils.TestClient

type TestFunction func(t *testing.T) (bool, bool)

func TestMain(m *testing.M) {

	kind := &unittestutils.KindCluster{}
	setUp := func() error {
		fmt.Println("Setup Cluster")
		err := kind.CreateCluster()
		kubecfgFlags := genericclioptions.NewConfigFlags(false)
		// create the K8s clientSet
		cfg, err := kubecfgFlags.ToRESTConfig()
		k8sClient, err := kubernetes.NewForConfig(cfg)

		if err != nil {
			return err
		}
		rest := k8sClient.CoreV1().RESTClient()
		var apiExtensions *apiextensionspkg.Clientset
		apiExtensions, err = apiextensionspkg.NewForConfig(cfg)
		testClient = unittestutils.TestNewClient("unittests", k8sClient, apiExtensions, rest, cfg)
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
		err := kind.DeleteCluster()
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
	g := gomega.NewGomegaWithT(t)
	tenantNamespace := "tenant1admin"
	serviceAccount := "t1-admin1"
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
		t.Logf("Running test: ", tc.testName)
		preRun, run := tc.testFunction(t)
		g.Expect(preRun).To(gomega.Equal(tc.preRun))
		g.Expect(run).To(gomega.Equal(tc.run))
	}
}

func testPreRunWithoutRole(t *testing.T) (preRun bool, run bool) {
	tenantConfig := testClient.Config
	tenantConfig.Impersonate.UserName = "system:serviceaccount:default:t1-admin1"
	tenantClient, _ := kubernetes.NewForConfig(tenantConfig)
	err := b.PreRun(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		t.Logf(err.Error())
		return false, false
	}
	return true, false

}

func testPreRunWithRole(t *testing.T) (preRun bool, run bool) {
	createdRole, err := testClient.CreateRole()
	_, err = testClient.CreateRoleBinding(createdRole)
	if err != nil {
		t.Logf(err.Error())
	}
	t.Logf("Roles are created")
	tenantConfig := testClient.Config
	tenantConfig.Impersonate.UserName = "system:serviceaccount:default:t1-admin1"
	tenantClient, _ := kubernetes.NewForConfig(tenantConfig)
	err = b.PreRun(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		return false, false
	} else {
		err = b.Run(testClient.Namespace, testClient.K8sClient, tenantClient)
		if err != nil {
			return true, false
		}
	}
	return true, true
}

func testRunWithPolicy(t *testing.T) (preRun bool, run bool) {
	path := filepath.Join("..", "..", "assets")
	crdPath := "https://github.com/nirmata/kyverno/raw/master/definitions/install.yaml"
	policyPath := filepath.Join(path, "policy.yaml")

	paths := []string{crdPath, policyPath}

	for _, p := range paths {

		err := testClient.CreatePolicy(p)
		if err != nil {
			t.Logf(err.Error())
			return false, false
		}
	}

	tenantConfig := testClient.Config
	tenantConfig.Impersonate.UserName = "system:serviceaccount:default:t1-admin1"
	tenantClient, _ := kubernetes.NewForConfig(tenantConfig)

	err := b.PreRun(testClient.Namespace, testClient.K8sClient, tenantClient)
	if err != nil {
		t.Logf(err.Error())
		return false, false
	} else {
		err = b.Run(testClient.Namespace, testClient.K8sClient, tenantClient)
		fmt.Println(err)
		if err != nil {
			return true, false
		}
	}
	return true, true
}
