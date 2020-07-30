package blockuseofbindmounts

import (
	"context"
	"fmt"
	"path/filepath"

	"os"
	"strings"
	"testing"

	"github.com/onsi/gomega"
	v1 "k8s.io/api/rbac/v1"
	apiextensionspkg "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/kind/pkg/cluster"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/unittestutils"
)

var (
	testClient    *unittestutils.TestClient
	tenantConfig  *rest.Config
	tenantClient  *kubernetes.Clientset
	clusterExists bool
	saName        = "admin"
	apiExtensions *apiextensionspkg.Clientset
	g             *gomega.GomegaWithT
	namespace     = "ns-" + string(uuid.NewUUID())[0:4]
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
		tenantConfig.Impersonate.UserName = "system:serviceaccount:" + namespace + ":" + saName
		tenantClient, _ = kubernetes.NewForConfig(tenantConfig)
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

func TestBenchmark(t *testing.T) {
	defer func() {
		testClient.DeletePolicy()
		err := testClient.K8sClient.CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
		if err != nil {
			g.Expect(err).NotTo(gomega.HaveOccurred())
		}
	}()

	g = gomega.NewGomegaWithT(t)

	testClient.Namespace = namespace
	_, err := testClient.K8sClient.CoreV1().Namespaces().Create(context.TODO(), unittestutils.NamespaceObj(namespace), metav1.CreateOptions{})
	if err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	testClient.Namespace = namespace
	_, err = testClient.K8sClient.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), unittestutils.ServiceAccountObj(saName, namespace), metav1.CreateOptions{})
	if err != nil {
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}
	testClient.ServiceAccount = unittestutils.ServiceAccountObj(saName, namespace)

		// Install Kyverno
	path := filepath.Join("..", "..", "assets")
	crdPath := filepath.Join(path, "kyverno.yaml")
	err = testClient.CreatePolicy(crdPath)
	if err != nil {
		fmt.Println(err.Error())
		g.Expect(err).NotTo(gomega.HaveOccurred())
	}

	err = unittestutils.WaitForKyvernoToReady(testClient.K8sClient)
	if err != nil {
		fmt.Println(err.Error())
		g.Expect(err).NotTo(gomega.HaveOccurred())
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
	paths := []string{unittestutils.DisallowBindMounts}

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
