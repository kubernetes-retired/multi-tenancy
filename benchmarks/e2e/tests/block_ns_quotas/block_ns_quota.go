package block_ns_quotas

import (
	"fmt"
	"github.com/onsi/ginkgo"
	configutil "github.com/realshuting/multi-tenancy/benchmarks/e2e/config"
	"k8s.io/kubernetes/test/e2e/framework"
	"os"
	"time"
)

const (
	expectedVal = "no"
)

var _ = framework.KubeDescribe("test tenant permission", func() {
	var config *configutil.BenchmarkConfig
	var tenantA configutil.TenantSpec
	var user string
	var err error

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		tenantA = config.GetValidTenant()
		os.Setenv("KUBECONFIG", tenantA.Kubeconfig)
		user = configutil.GetContextFromKubeconfig(tenantA.Kubeconfig)
	})

	ginkgo.It("delete resource quota", func() {
		ginkgo.By(fmt.Sprintf("tenant %s cannot delete resource quota", user))
		nsFlag := fmt.Sprintf("--namespace=%s", tenantA.Namespace)
		verbs := []string{"get", "update", "patch", "delete", "deletecollection"}
		for _, verb := range verbs {
			_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
				_, err := framework.RunKubectl("auth","can-i", verb, "quota", nsFlag)
				return err.Error()
			})

			framework.ExpectNoError(errNew)
		}
	})
})