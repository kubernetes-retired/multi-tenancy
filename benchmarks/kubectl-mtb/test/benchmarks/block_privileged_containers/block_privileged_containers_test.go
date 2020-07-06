package blockprivilegedcontainers

import (
	"os"
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
)

func setupTest(t *testing.T) func(t *testing.T) {
	t.Log("Setup Cluster")
	k := &utils.KindCluster{}
	k.CreateCluster()
	return func(t *testing.T) {
		t.Log("Teardown Cluster")
		k.DeleteCluster()
	}
}

func TestTenant(t *testing.T) {
	g := gomega.NewGomegaWithT(t)
	teardownTest := setupTest(t)
	defer teardownTest(t)
	utils.CreateCrds()
	utils.CreateTenant(t, g)
	cases := []struct {
		testName        string
		tenantNamespace string
		tenant          string
	}{
		{"PreRun", "tenant1admin", "tenant1-sample"},
	}
	kubecfgFlags := genericclioptions.NewConfigFlags(false)
	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		t.Logf(err.Error())
	}
	// create the K8s clientset
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		t.Logf(err.Error())
	}
	tenantConfig := config
	for _, tc := range cases {
		t.Run("PreRun", func(t *testing.T) {
			tenantConfig.Impersonate.UserName = tc.tenant
			// create the tenant clientset
			tenantClient, err := kubernetes.NewForConfig(tenantConfig)
			if err != nil {
				t.Logf(err.Error())
			}
			err = BpcBenchmark.PreRun(tc.tenantNamespace, k8sClient, tenantClient)
			if err != nil {
				t.Logf(err.Error())
				os.Exit(1)
			}
		})
	}
}
