package test

import (
	"fmt"
	"strings"

	"github.com/onsi/ginkgo"
	configutil "github.com/realshuting/multi-tenancy/benchmarks/e2e/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	expectedVal = "Error from server (Forbidden)"
)

var _ = framework.KubeDescribe("A tenant namespace must have resource quotas", func() {
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

	ginkgo.It("validate resource quotas are configured", func() {
		ginkgo.By(fmt.Sprintf("tenant %s namespace must have resource quotas configured", user))
		resourceNameList := getResourceNameList(config.Adminkubeconfig)
		tenantResourcequotas := getTenantResoureQuotas(tenantA)
		expectedVal := strings.Join(tenantResourcequotas, " ")
		for _, r := range resourceNameList {
			if !strings.Contains(expectedVal, r) {
				framework.Failf("%s must be configured in tenant %s namespace resource quotas", r, user)
			}
		}
	})
})

func getTenantResoureQuotas(t configutil.TenantSpec) []string {
	var tmpList string
	var tenantResourceQuotas []string

	kclient := configutil.NewKubeClientWithKubeconfig(t.Kubeconfig)
	resourcequotaList, err := kclient.CoreV1().ResourceQuotas(t.Namespace).List(metav1.ListOptions{})
	framework.ExpectNoError(err)

	for _, resourcequota := range resourcequotaList.Items {
		for name := range resourcequota.Spec.Hard {
			if strings.Contains(tmpList, name.String()) {
				continue
			}

			tenantResourceQuotas = append(tenantResourceQuotas, name.String())
			tmpList = tmpList + name.String()
		}
	}

	return tenantResourceQuotas
}

func getResourceNameList(kubeconfigpath string) []string {
	kclient := configutil.NewKubeClientWithKubeconfig(kubeconfigpath)
	nodes, err := kclient.CoreV1().Nodes().List(metav1.ListOptions{})
	framework.ExpectNoError(err)

	return getResourcequotaFromNodes(*nodes)
}

func getResourcequotaFromNodes(nodeList corev1.NodeList) []string {
	var resourceNameList []string
	var tmpList string
	for _, node := range nodeList.Items {
		for resourceName := range node.Status.Capacity {
			if strings.Contains(tmpList, resourceName.String()) {
				continue
			}

			resourceNameList = append(resourceNameList, resourceName.String())
			tmpList = tmpList + resourceName.String()
		}
	}
	return resourceNameList
}
