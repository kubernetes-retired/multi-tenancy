package configurensobjectquota

import (
	"fmt"
	"strings"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
)

var b = &benchmark.Benchmark{
	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		return nil
	},
	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		resourceNameList := [9]string{"pods", "services", "replicationcontrollers", "resourcequotas", "secrets", "configmaps", "persistentvolumeclaims", "services.nodeports", "services.loadbalancers"}
		tenantResourceQuotas := utils.GetTenantResoureQuotas(tenantNamespace, kclient)
		expectedVal := strings.Join(tenantResourceQuotas, " ")
		for _, r := range resourceNameList {
			if !strings.Contains(expectedVal, r) {
				return fmt.Errorf("%s must be configured in %s resource quotas", r, tenantNamespace)
			}
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("configure_ns_object_quota/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
