package configure_ns_object_quotas

import (
	"fmt"
	"strings"

	"github.com/onsi/ginkgo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/test/e2e/framework"
	configutil "sigs.k8s.io/multi-tenancy/benchmarks/e2e/config"
)

const (
	expectedVal = "Error from server (Forbidden)"
)

var _ = framework.KubeDescribe("A tenant namespace must have object resource quotas", func() {
	var config *configutil.BenchmarkConfig
	var tenantA configutil.TenantSpec
	var user string
	var err error
	resourceNameList := [9]string{"pods", "services", "replicationcontrollers", "resourcequotas", "secrets", "configmaps", "persistentvolumeclaims", "services.nodeports", "services.loadbalancers"}

	ginkgo.BeforeEach(func() {
		config, err = configutil.ReadConfig(configutil.ConfigPath)
		framework.ExpectNoError(err)

		tenantA, err = config.GetValidTenant()
		framework.ExpectNoError(err)
		user = configutil.GetContextFromKubeconfig(tenantA.Kubeconfig)
	})

	ginkgo.It("validate object resource quotas are configured", func() {
		ginkgo.By(fmt.Sprintf("tenant %s namespace must have object resource quotas configured", user))
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