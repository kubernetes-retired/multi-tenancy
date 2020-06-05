package create_network_policies

import (
	"fmt"
	"os"
	"time"

	"github.com/onsi/ginkgo"
	"k8s.io/kubernetes/test/e2e/framework"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
)

const (
	expectedVal = "yes"
)

var _ = framework.KubeDescribe("[PL2] [PL3] Test tenant's network-policy management permissions", func() {
	var config *configutil.BenchmarkConfig
	var tenantkubeconfig configutil.TenantSpec
	var err error

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)
	})

	framework.KubeDescribe("Tenant has RBAC privileges for Network-policies", func() {
		var user string
		var verbs = []string{"get", "list", "create", "update", "patch", "watch", "delete", "deletecollection"}
		var namespaceflag = "-n"

		ginkgo.BeforeEach(func() {
			tenantkubeconfig, err = config.GetValidTenant()
			framework.ExpectNoError(err)

			os.Setenv("KUBECONFIG", tenantkubeconfig.Kubeconfig)
			user = configutil.GetContextFromKubeconfig(tenantkubeconfig.Kubeconfig)
		})

		ginkgo.It("Tenant has RBAC privileges for Network-policies", func() {
			ginkgo.By(fmt.Sprintf("Tenant %s can modify Network-policies for its namespace", user))

			for _, verb := range verbs {
				_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
					output, err := framework.RunKubectl("auth", "can-i", verb, "networkpolicy", namespaceflag, tenantkubeconfig.Namespace)
					if err != nil {
						return err.Error()
					}
					return output
				})

				framework.ExpectNoError(errNew)
			}
		})
	})
})
