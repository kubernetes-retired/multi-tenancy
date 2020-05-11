package block_privileged_containers

import (
	"fmt"
	"strings"

	"github.com/onsi/ginkgo"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
)

const (
	expectedVal = "Privileged containers are not allowed"
)

var _ = framework.KubeDescribe("[PL1] [PL2] [PL3] Tenants should not be allowed to run privileged containers", func() {
	var config *configutil.BenchmarkConfig
	var tenantA configutil.TenantSpec
	var user string
	var err error

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		tenantA, err = config.GetValidTenant()
		framework.ExpectNoError(err)
		user = configutil.GetContextFromKubeconfig(tenantA.Kubeconfig)
	})

	ginkgo.It("validate tenants can't create privileged containers", func() {
		ginkgo.By(fmt.Sprintf("tenant %s cannot create pod/container with securityContext.privileged set to true",
			user))
		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)
		// IsPrivileged set to true so that pod creation would fail
		pod := e2epod.MakeSecPod(tenantA.Namespace, nil, nil, true, "", false, false, nil, nil)
		_, err = kclient.CoreV1().Pods(tenantA.Namespace).Create(pod)
		if !strings.Contains(err.Error(), expectedVal) {
			framework.Failf("%s must be unable to create pod that sets privileged to true", user)
		}
	})
})
