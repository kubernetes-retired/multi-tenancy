package test

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/onsi/ginkgo"
	configutil "github.com/realshuting/multi-tenancy/benchmarks/e2e/config"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	expectedVal = "Error from server (Forbidden)"
)

var _ = framework.KubeDescribe("test cross tenants permission", func() {
	var config *configutil.BenchmarkConfig
	var resourceList string
	var err error
	var tenantA, tenantB string
	namespaceFlag := fmt.Sprintf("-n")

	ginkgo.BeforeEach(func() {
		ginkgo.By("get namespace wide api-resources")

		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		os.Setenv("KUBECONFIG", config.TenantA.Kubeconfig)
		tenantA = configutil.GetContextFromKubeconfig(config.TenantA.Kubeconfig)
		outputFlag := fmt.Sprintf("-o=name")

		resourceList, err = framework.RunKubectl(namespaceFlag, config.TenantA.Namespace, "api-resources", outputFlag)
		framework.ExpectNoError(err)
	})

	framework.KubeDescribe("tenant cannot access other tenant namespaced resources", func() {

		ginkgo.BeforeEach(func() {
			os.Setenv("KUBECONFIG", config.TenantB.Kubeconfig)
			tenantB = configutil.GetContextFromKubeconfig(config.TenantB.Kubeconfig)
		})

		ginkgo.It("get namespaced resources", func() {
			ginkgo.By(fmt.Sprintf("tenant %s cannot get tenant %s namespaced resources", tenantB, tenantA))
			resources := strings.Fields(resourceList)
			for _, resource := range resources {
				_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
					_, err := framework.RunKubectl(namespaceFlag, config.TenantA.Namespace, "get", resource)
					return err.Error()
				})

				framework.ExpectNoError(errNew)
			}
		})
	})
})
