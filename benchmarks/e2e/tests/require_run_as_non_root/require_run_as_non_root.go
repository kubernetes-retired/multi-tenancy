package tenantaccess

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strings"
	"github.com/onsi/ginkgo"

	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
	imageutils "k8s.io/kubernetes/test/utils/image"

)

const (
	expectedVal = "securityContext.runAsNonRoot: Invalid value: false: must be true"
)

var _ = framework.KubeDescribe("Tenants should required to run containers/pods as NonRoot", func() {
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

	ginkgo.It("validate tenants can't create containers with RunAsNonRoot set to false", func() {
		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)
		BusyBoxImage := imageutils.GetE2EImage(imageutils.BusyBox)
		setFalse := false
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
				SecurityContext: &v1.PodSecurityContext{
					// RunAsNonRoot set to false so that pod creation would fail
					RunAsNonRoot: &setFalse,
				},
				Containers: []v1.Container{
					{
						Name:    "write-pod",
						Image:   BusyBoxImage,
						Command: []string{"/bin/sh"},
						Args:    []string{"-c", "trap exit TERM; while true; do sleep 1; done"},
					},
				},
			},
		}
		_, err = kclient.CoreV1().Pods(tenantA.Namespace).Create(podSpec)
		if !strings.Contains(err.Error(),expectedVal) {
			framework.Failf("%s must be unable to create pod with HostPID set to true", user)
		}
	})
})