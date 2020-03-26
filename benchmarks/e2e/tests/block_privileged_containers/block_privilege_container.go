package block_privileged_containers

import (
	"fmt"
	"strings"

	"github.com/onsi/ginkgo"
	configutil "github.com/realshuting/multi-tenancy/benchmarks/e2e/config"
	"k8s.io/kubernetes/test/e2e/framework"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
)

const (
	expectedVal = "Privileged containers are not allowed"
)

var _ = framework.KubeDescribe("Tenants should not be allowed to run privileged containers", func() {
	var config *configutil.BenchmarkConfig
	var tenantA configutil.TenantSpec
	var user string
	var err error

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		tenantA = config.GetValidTenant()
		user = configutil.GetContextFromKubeconfig(tenantA.Kubeconfig)
	})

	ginkgo.It("validate tenants can't create privileged containers", func() {
		ginkgo.By(fmt.Sprintf("tenant ${user} cannot create pod/container with securityContext.privileged set to true",
			user))
		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)
		// IsPrivileged set to true so that pod creation would fail
		pod := e2epod.MakeSecPod(tenantA.Namespace, nil, nil, true, "", false, false, nil, nil)
		_, err = kclient.CoreV1().Pods(tenantA.Namespace).Create(pod)
		if !strings.Contains(err.Error(),expectedVal) {
			framework.Failf("%s must be unable to create pod with HostPID set to true", user)
		}
	})
})