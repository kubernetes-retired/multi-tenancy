package block_bind_mounts

import (
	"fmt"
	"strings"

	"github.com/onsi/ginkgo"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
	"k8s.io/kubernetes/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
)

const (
	expectedVal = "Host path volumes are not allowed"
)

var _ = framework.KubeDescribe("Tenants should not be able to mount host volumes and folders", func() {
	var config *configutil.BenchmarkConfig
	var tenantA configutil.TenantSpec
	var user string
	var err error
	var InlineVolumeSources = []*v1.VolumeSource{
		{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/tmp/busybox",
			},
		},
	}

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		tenantA, err = config.GetValidTenant()
		framework.ExpectNoError(err)

		user = configutil.GetContextFromKubeconfig(tenantA.Kubeconfig)
	})

	ginkgo.It("Tenants should not be able to mount host volumes and folders", func() {
		ginkgo.By(fmt.Sprintf("Tenant %s should not be able to mount host volumes and folders", user))
				
		pod := e2epod.MakeSecPod(tenantA.Namespace, nil,  InlineVolumeSources, false, "", false, false, nil, nil)

		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)
		_, err = kclient.CoreV1().Pods(tenantA.Namespace).Create(pod)

		if !strings.Contains(err.Error(), expectedVal) {
			framework.Failf("%s must be unable to create pod with host-path volume", user)
		}
	})
})

