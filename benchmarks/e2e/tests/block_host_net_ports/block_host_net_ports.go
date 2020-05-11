package block_host_net_ports

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/kubernetes/test/e2e/framework"
	imageutils "k8s.io/kubernetes/test/utils/image"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
)

const (
	expectedHostNetworkVal = "Host network is not allowed to be used"
	expectedHostPortVal    = "Host port is not allowed to be used"
)

func MakeSecPod(Namespace string, HostNetwork bool, Ports []v1.ContainerPort) *v1.Pod {
	podName := "security-context-" + string(uuid.NewUUID())
	podSpec := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: Namespace,
		},
		Spec: v1.PodSpec{
			HostNetwork: HostNetwork,
			Containers: []v1.Container{
				{
					Name:    "write-pod",
					Image:   imageutils.GetE2EImage(imageutils.BusyBox),
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", ""},
					Ports:   Ports,
				},
			},
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}
	return podSpec
}

var _ = framework.KubeDescribe("[PL1] [PL2] [PL3] Host network is not allowed to be used", func() {
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

	ginkgo.It("Tenants should not be allowed to use host networking", func() {
		ginkgo.By(fmt.Sprintf("Tenant %s containers cannot use host networking", user))
		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)
		pod := MakeSecPod(tenantA.Namespace, true, nil)

		_, err = kclient.CoreV1().Pods(tenantA.Namespace).Create(pod)
		if !strings.Contains(err.Error(), expectedHostNetworkVal) {
			framework.Failf("%s must be unable to create pod with host networking set to true", user)
		}
	})
})

var _ = framework.KubeDescribe("[PL1] [PL2] [PL3] Host ports is not allowed to be used", func() {
	var config *configutil.BenchmarkConfig
	var tenantA configutil.TenantSpec
	var user string
	var err error
	var ports = []v1.ContainerPort{
		{
			HostPort:      8086,
			ContainerPort: 8086,
		},
	}

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		tenantA, err = config.GetValidTenant()
		framework.ExpectNoError(err)

		user = configutil.GetContextFromKubeconfig(tenantA.Kubeconfig)
	})

	ginkgo.It("Tenants should not be allowed to use host ports", func() {
		ginkgo.By(fmt.Sprintf("Tenant %s containers cannot use host ports", user))
		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)

		pod := MakeSecPod(tenantA.Namespace, false, ports)

		_, err = kclient.CoreV1().Pods(tenantA.Namespace).Create(pod)

		reg, regerr := regexp.Compile(`(\s|^)\d+`)
		if err != nil {
			framework.ExpectNoError(regerr)
		}
		// Removed digits from the error to match the expectedVal using regex
		processedError := reg.ReplaceAllString(err.Error(), "")

		if !strings.Contains(processedError, expectedHostPortVal) {
			framework.Failf("%s must be unable to create pod with defined host ports", user)
		}
	})
})
