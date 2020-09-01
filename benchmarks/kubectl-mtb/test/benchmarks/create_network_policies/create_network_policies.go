package createnetworkpolicies

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/log"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"
)

var b = &benchmark.Benchmark{

	PreRun: func(options types.RunOptions) error {

		return nil
	},
	Run: func(options types.RunOptions) error {
		verbs := []string{"get", "create", "update", "patch", "delete", "deletecollection"}

		resource := utils.GroupResource{
			APIGroup: "networking.k8s.io",
			APIResource: metav1.APIResource{
				Name: "networkpolicies",
			},
		}

		for _, verb := range verbs {
			access, msg, err := utils.RunAccessCheck(options.TClient, options.TenantNamespace, resource, verb)
			if err != nil {
				log.Logging.Debug(err.Error())
				return err
			}
			if !access {
				return fmt.Errorf(msg)
			}
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("create_network_policies/config.yaml"))
	if err != nil {
		log.Logging.Error(err.Error())
	}

	test.BenchmarkSuite.Add(b)
}
