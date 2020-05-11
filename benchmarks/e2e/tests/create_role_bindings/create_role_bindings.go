package create_role_bindings

import (
	"fmt"
	"os"
	"time"

	"github.com/onsi/ginkgo"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/test/e2e/framework"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
)

const (
	expectedVal = "yes"
)

var _ = framework.KubeDescribe("[PL2] [PL3] Test tenant's role management permissions", func() {
	var config *configutil.BenchmarkConfig
	var tenantkubeconfig configutil.TenantSpec
	var err error
	var roleName = "role-" + string(uuid.NewUUID())
	var rolebindingName = "rolebinding-" + string(uuid.NewUUID())
	var rolenameFlag = "--role=" + roleName

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)
	})

	framework.KubeDescribe("Tenant has RBAC privileges for Roles and Rolebindings", func() {
		var user string
		var verbs = []string{"get", "list", "create", "update", "patch", "watch", "delete", "deletecollection"}
		var namespaceflag = "-n"

		ginkgo.BeforeEach(func() {
			tenantkubeconfig, err = config.GetValidTenant()
			framework.ExpectNoError(err)

			os.Setenv("KUBECONFIG", tenantkubeconfig.Kubeconfig)
			user = configutil.GetContextFromKubeconfig(tenantkubeconfig.Kubeconfig)
		})

		ginkgo.It("Tenant has RBAC privileges for Roles", func() {
			ginkgo.By(fmt.Sprintf("Tenant %s can modify Roles in its namespace", user))

			for _, verb := range verbs {
				_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
					output, err := framework.RunKubectl("auth", "can-i", verb, "role", namespaceflag, tenantkubeconfig.Namespace)
					if err != nil {
						return err.Error()
					}
					return output
				})

				framework.ExpectNoError(errNew)
			}
		})

		ginkgo.It("Tenant has RBAC privileges for Role-bindings", func() {
			ginkgo.By(fmt.Sprintf("Tenant %s can modify Role-bindings in its namespace", user))

			for _, verb := range verbs {
				_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
					output, err := framework.RunKubectl("auth", "can-i", verb, "rolebinding", namespaceflag, tenantkubeconfig.Namespace)
					if err != nil {
						return err.Error()
					}
					return output
				})

				framework.ExpectNoError(errNew)
			}
		})

		ginkgo.It("Tenant can create rolebinding to a role", func() {
			ginkgo.By(fmt.Sprintf("Tenant %s can create rolebinding to a role", user))

			verb := "--verb=get"
			resource := "--resource=pods"

			_, err := framework.RunKubectl("create", "role", roleName, verb, resource, namespaceflag, tenantkubeconfig.Namespace)
			framework.ExpectNoError(err)

			_, errNew := framework.RunKubectl("create", "rolebinding", rolebindingName, rolenameFlag, namespaceflag, tenantkubeconfig.Namespace)
			framework.ExpectNoError(errNew)

		})
	})
})
