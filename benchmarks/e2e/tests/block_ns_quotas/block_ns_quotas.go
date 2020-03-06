package test

import (
	"fmt"
	"os"
	"time"

	"github.com/onsi/ginkgo"
	configutil "github.com/realshuting/multi-tenancy/benchmarks/e2e/config"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	expectedVal = "No"
)

var _ = framework.KubeDescribe("test resource quotas modification permissions", func() {
	var config *configutil.BenchmarkConfig
	var err error
	// var dryrun = "--dry-run=true"
	// var all = "--all=true"
	actions := [5]string{"create", "update", "patch", "delete", "deletecollection"}

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)
	})

	framework.KubeDescribe("tenant cannnot modify resource quotas", func() {
		var user string

		ginkgo.BeforeEach(func() {
			tenantkubeconfig := config.GetValidTenant()
			os.Setenv("KUBECONFIG", tenantkubeconfig.Kubeconfig)
			user = configutil.GetContextFromKubeConfig(tenantkubeconfig.Kubeconfig)
		})

		ginkgo.It("modify resource quotas", func() {
			ginkgo.By(fmt.Sprintf("tenant %s cannot modify resource quotas", user))
			for _, action := range actions {
				_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
					_, err := framework.RunKubectl("-n", "tenant1admin", "auth can-i", action, "quota")
					return err.Error()
				})

				framework.ExpectNoError(errNew)
			}
		})
	})
})
