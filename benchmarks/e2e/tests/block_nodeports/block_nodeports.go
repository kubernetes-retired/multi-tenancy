package block_nodeports

import (
	"fmt"
	"strings"

	"github.com/onsi/ginkgo"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
	"k8s.io/kubernetes/test/e2e/framework"
	v1 "k8s.io/api/core/v1"
	e2edeployment "k8s.io/kubernetes/test/e2e/framework/deployment"
	"k8s.io/apimachinery/pkg/util/uuid"
	imageutils "k8s.io/kubernetes/test/utils/image"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	expectedVal = "Services of type NodePort are not allowed"
)

func CreateServiceSpec(serviceName string, selector map[string]string) *v1.Service {
	Service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceName,
		},
		Spec: v1.ServiceSpec{
			Selector: selector,
		},
	}
	Service.Spec.Type = "NodePort"
	Service.Spec.Ports = []v1.ServicePort{
		{Port: 80, Name: "http", Protocol: v1.ProtocolTCP},
	}
	return Service
}

var _ = framework.KubeDescribe("Tenants should not be able to create services of type NodePort.", func() {
	var config *configutil.BenchmarkConfig
	var tenantA configutil.TenantSpec
	var user string
	var err error
	var deploymentName string
	var imageName string
	var podLabels = map[string]string {"test": "multi"}
	var serviceName string

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		tenantA, err = config.GetValidTenant()
		framework.ExpectNoError(err)

		user = configutil.GetContextFromKubeconfig(tenantA.Kubeconfig)
		deploymentName = "deployment-" + string(uuid.NewUUID())
		imageName = "image-" + string(uuid.NewUUID())
		serviceName = "image-" + string(uuid.NewUUID())
	})

	ginkgo.It("Tenants should not be able to create services of type NodePort.", func() {
		ginkgo.By(fmt.Sprintf("Tenant %s should not be able to create services of type NodePort.", user))

		deployment := e2edeployment.NewDeployment(deploymentName, 1, podLabels, imageName, imageutils.GetE2EImage(imageutils.Nginx), "Recreate")

		kclient := configutil.NewKubeClientWithKubeconfig(tenantA.Kubeconfig)
		_, err = kclient.AppsV1().Deployments(tenantA.Namespace).Create(deployment)
		framework.ExpectNoError(err)

		svc := CreateServiceSpec(serviceName, podLabels)
		_, err = kclient.CoreV1().Services(tenantA.Namespace).Create(svc)

		if !strings.Contains(err.Error(), expectedVal) {
			framework.Failf("%s must be unable to create service of type NodePort", user)
		}
	})
})