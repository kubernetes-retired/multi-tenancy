package block_privilege_escalation

import (
	"fmt"
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
	expectedVal = "Allowing privilege escalation for containers is not allowed"
)

func MakeSecPod(Namespace string, AllowPrivilegeEscalation bool) *v1.Pod {
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
			Containers: []v1.Container{
				{
					Name:    "write-pod",
					Image:   imageutils.GetE2EImage(imageutils.BusyBox),
					Command: []string{"/bin/sh"},
					Args:    []string{"-c", ""},
					SecurityContext: &v1.SecurityContext{
						AllowPrivilegeEscalation: &AllowPrivilegeEscalation,
					},
				},
			},
			RestartPolicy: v1.RestartPolicyOnFailure,
		},
	}
	return podSpec
}

var _ = framework.KubeDescribe("[PL1] [PL2] [PL3] Processes in tenant containers should not be allowed to gain additional priviliges", func() {
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

	ginkgo.It("Validate tenants can not create pods/container with allowedprivilege set to true", func() {
		ginkgo.By(fmt.Sprintf("tenant %s cannot create pod/container with with allowedprivilege set to true", user))

		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)

		pod := MakeSecPod(tenantA.Namespace, true)
		_, err = kclient.CoreV1().Pods(tenantA.Namespace).Create(pod)

		if !strings.Contains(err.Error(), expectedVal) {
			framework.Failf("%s must be unable to create pod/container that sets allowedprivileged to true", user)
		}
	})
})
