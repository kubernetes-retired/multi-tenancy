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

var _ = framework.KubeDescribe("test tenant's permission to modify multi-tenancy resources", func() {
	var config *configutil.BenchmarkConfig
	var resourceList string
	var err error
	var dryrun = "--dry-run=true"
	var nsFlag = "-n"
	var outputFlag = "-o=name"

	ginkgo.BeforeEach(func() {
		ginkgo.By("get resources managed by the cluster administrator in tenant's namespace")

		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		os.Setenv("KUBECONFIG", config.Adminkubeconfig)
		labelFlg := fmt.Sprintf("-l")

		tenantkubeconfig := config.GetValidTenant()

		resourceList, err = framework.RunKubectl("get", "all", labelFlg, config.Label , nsFlag, tenantkubeconfig.Namespace, outputFlag)
		framework.ExpectNoError(err)
	})

	framework.KubeDescribe("tenant cannot modify resources managed by the cluster administrator in tenant's namespace", func() {
		var user string

		ginkgo.BeforeEach(func() {
			tenantkubeconfig := config.GetValidTenant()
			os.Setenv("KUBECONFIG", tenantkubeconfig.Kubeconfig)
			user = configutil.GetContextFromKubeconfig(tenantkubeconfig.Kubeconfig)
		})

		ginkgo.It("annotate resources managed by the cluster administrator in tenant's namespace", func() {
			ginkgo.By(fmt.Sprintf("tenant %s cannot annotate resources managed by the cluster administrator in tenant's namespace", user))
			resources := strings.Fields(resourceList)
			annotate := fmt.Sprintf("test=multi")
			tenantkubeconfig := config.GetValidTenant()
			for _, resource := range resources {
				_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
					_, err := framework.RunKubectl(dryrun, nsFlag, tenantkubeconfig.Namespace, "annotate", resource, annotate)
					return err.Error()
				})

				framework.ExpectNoError(errNew)
			}
		})
	})
})

var _ = framework.KubeDescribe("test tenant's permission to modify multi-tenancy rolebindings", func() {
	var config *configutil.BenchmarkConfig
	var resourceList string
	var err error
	var dryrun = "--dry-run=true"
	var nsFlag = "-n"
	var outputFlag = "-o=name"

	ginkgo.BeforeEach(func() {
		ginkgo.By("get rolebindings managed by the cluster administrator in tenant's namespace")

		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		os.Setenv("KUBECONFIG", config.Adminkubeconfig)

		tenantkubeconfig := config.GetValidTenant()

		resourceList, err = framework.RunKubectl("get", "rolebinding", nsFlag, tenantkubeconfig.Namespace, outputFlag)
		framework.ExpectNoError(err)
	})

	framework.KubeDescribe("tenant cannot modify rolebinding in tenant's namespace", func() {
		var user string

		ginkgo.BeforeEach(func() {
			tenantkubeconfig := config.GetValidTenant()
			os.Setenv("KUBECONFIG", tenantkubeconfig.Kubeconfig)
			user = configutil.GetContextFromKubeconfig(tenantkubeconfig.Kubeconfig)
		})

		ginkgo.It("annotate rolebinding in tenant's namespace", func() {
			ginkgo.By(fmt.Sprintf("tenant %s cannot annotate rolebinding in tenant's namespace", user))
			resources := strings.Fields(resourceList)
			annotate := fmt.Sprintf("test=multi")
			tenantkubeconfig := config.GetValidTenant()
			for _, resource := range resources {
				_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
					_, err := framework.RunKubectl(dryrun, nsFlag, tenantkubeconfig.Namespace, "annotate", resource, annotate)
					return err.Error()
				})

				framework.ExpectNoError(errNew)
			}
		})
	})
})
	