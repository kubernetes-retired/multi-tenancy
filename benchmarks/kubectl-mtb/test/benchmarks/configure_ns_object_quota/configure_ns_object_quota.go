package configurensobjectquota

import (
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"
)

var b = &benchmark.Benchmark{
	PreRun: func(options types.RunOptions) error {

		verbs := []string{"list", "get"}

		for _, verb := range verbs {
			resource := utils.GroupResource{
				APIGroup: "",
				APIResource: metav1.APIResource{
					Name: "resourcequotas",
				},
			}

			access, msg, err := utils.RunAccessCheck(options.TenantClient, options.TenantNamespace, resource, verb)
			if err != nil {
				options.Logger.Debug(err.Error())
				return err
			}
			if !access {
				return fmt.Errorf(msg)
			}
		}

		return nil
	},
	Run: func(options types.RunOptions) error {

		resourceNameList := [9]string{"pods", "services", "replicationcontrollers", "resourcequotas", "secrets", "configmaps", "persistentvolumeclaims", "services.nodeports", "services.loadbalancers"}
		tenantResourceQuotas := utils.GetTenantResoureQuotas(options.TenantNamespace, options.TenantClient)
		expectedVal := strings.Join(tenantResourceQuotas, " ")
		for _, r := range resourceNameList {
			if !strings.Contains(expectedVal, r) {
				return fmt.Errorf("%s must be configured in %s resource quotas", r, options.TenantNamespace)
			}
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("configure_ns_object_quota/config.yaml"))
	if err != nil {
		fmt.Println(err.Error())
	}

	test.BenchmarkSuite.Add(b)
}
