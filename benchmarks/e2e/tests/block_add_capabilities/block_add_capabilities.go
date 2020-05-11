package tenantaccess

import (
	"strings"

	"github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	imageutils "k8s.io/kubernetes/test/utils/image"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
)

const (
	expectedVal = "capability may not be added"
)

var _ = framework.KubeDescribe("[PL1] [PL2] [PL3] Tenants should unable to add linux capabilities for pods", func() {
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

	ginkgo.It("validate tenants can't create containers with add capabilities", func() {
		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)
		BusyBoxImage := imageutils.GetE2EImage(imageutils.BusyBox)
		podSpec := &v1.Pod{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Pod",
				APIVersion: "v1",
			},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "multitenant-tester",
				Namespace:    tenantA.Namespace,
			},
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					{
						Name:    "write-pod",
						Image:   BusyBoxImage,
						Command: []string{"/bin/sh"},
						Args:    []string{"-c", "trap exit TERM; while true; do sleep 1; done"},
						SecurityContext: &v1.SecurityContext{
							Capabilities: &v1.Capabilities{
								Add: []v1.Capability{
									"SETPCAP",
								},
							},
						},
					},
				},
			},
		}
		_, err = kclient.CoreV1().Pods(tenantA.Namespace).Create(podSpec)
		if !strings.Contains(err.Error(), expectedVal) {
			framework.Failf("%s must be unable to create pod with add capabilities", user)
		}
	})
})
