package default_deny_net_conn

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
	"k8s.io/kubernetes/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	imageutils "k8s.io/kubernetes/test/utils/image"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	"k8s.io/apimachinery/pkg/util/uuid"
)

const (
	expectedVal = "command terminated with exit code 1"
)

func MakeSpecPod(name string, Namespace string) (*v1.Pod) {
	podSpec := &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: Namespace,
			Labels: map[string]string {"run": "my-nginx"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:    name,
					Image:   imageutils.GetE2EImage(imageutils.Nginx),
				},
			},
			RestartPolicy: v1.RestartPolicyAlways,
		},
	}
	return podSpec
}

func CreateServiceSpec(serviceName, externalName string, isHeadless bool, selector map[string]string) *v1.Service {
	headlessService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
			Labels: map[string]string {"run": "my-nginx"},
		},
		Spec: v1.ServiceSpec{
			Selector: selector,
		},
	}
	if externalName != "" {
		headlessService.Spec.Type = v1.ServiceTypeExternalName
		headlessService.Spec.ExternalName = externalName
	} else {
		headlessService.Spec.Ports = []v1.ServicePort{
			{Port: 80, Name: "http", Protocol: v1.ProtocolTCP},
		}
	}
	if isHeadless {
		headlessService.Spec.ClusterIP = "None"
	}
	return headlessService
}


var _ = framework.KubeDescribe("Tenants should have explicit control over ingress connections for their workloads", func() {
	var config *configutil.BenchmarkConfig
	var tenantA, tenantB string
	var namespaceFlag = "-n"
	var err error
	var labels = map[string]string {"run": "my-nginx"}
	var name = "security-context-" + string(uuid.NewUUID())
	var url string

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		err = config.ValidateTenant(config.TenantA)
		framework.ExpectNoError(err)

		err = config.ValidateTenant(config.TenantB)
		framework.ExpectNoError(err)

		tenantA = configutil.GetContextFromKubeconfig(config.TenantA.Kubeconfig)
		tenantB = configutil.GetContextFromKubeconfig(config.TenantB.Kubeconfig)

		url = "http://" + name + "." + config.TenantA.Namespace
	})

	ginkgo.It("Tenant cannot connect to the pod or services of other tenant", func() {
		ginkgo.By(fmt.Sprintf("Tenant %s cannot connect to the service in the %s namespace", tenantB, tenantA))
		
		kclientTenantA := configutil.NewKubeClientWithKubeconfig(config.TenantA.Kubeconfig)

		// Making nginx pod in TenantA
		pod := MakeSpecPod(name, config.TenantA.Namespace)
		_, err = kclientTenantA.CoreV1().Pods(config.TenantA.Namespace).Create(pod)
		framework.ExpectNoError(err)
		
		// Making a service in TenantA to expose the nginx pod
		svc := CreateServiceSpec(name, "", false, labels)
		_, err = kclientTenantA.CoreV1().Services(config.TenantA.Namespace).Create(svc)
		framework.ExpectNoError(err)

		kclientTenantB := configutil.NewKubeClientWithKubeconfig(config.TenantB.Kubeconfig)
		
		// Making busybox pod in TenantB to connect to service in TenantA
		testpod := e2epod.MakeSecPod(config.TenantB.Namespace, nil, nil, false, "", false, false, nil, nil)
		_, err = kclientTenantB.CoreV1().Pods(config.TenantB.Namespace).Create(testpod)
		framework.ExpectNoError(err)

		// Wget the service exposed Url from the TenantB pod bash
		_, errNew := framework.LookForString(expectedVal, time.Minute, func() string {
			_, err := framework.RunKubectl(namespaceFlag, config.TenantB.Namespace, "exec", "-it", testpod.ObjectMeta.Name, "--", "wget" ,"--timeout=5" ,"-O" ,"-", url)
			return err.Error()
		})
		framework.ExpectNoError(errNew)
	})
})
