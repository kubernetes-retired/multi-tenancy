package blockothertenantresources

import (
	"fmt"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"

	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
)

var verbs = []string{"get", "update"}

var b = &benchmark.Benchmark{

	PreRun: func(options types.RunOptions) error {

		return nil
	},

	Run: func(options types.RunOptions) error {
		var resources []utils.GroupResource

		lists, err := options.KClient.Discovery().ServerPreferredResources()
		if err != nil {
			options.Logger.Debug(err.Error())
			return err
		}

		for _, list := range lists {
			if len(list.APIResources) == 0 {
				continue
			}
			gv, err := schema.ParseGroupVersion(list.GroupVersion)
			if err != nil {
				continue
			}
			for _, resource := range list.APIResources {
				if len(resource.Verbs) == 0 {
					continue
				}

				if !resource.Namespaced {
					continue
				}
				resources = append(resources, utils.GroupResource{
					APIGroup:    gv.Group,
					APIResource: resource,
				})
			}
		}

		err = utils.CheckAccessOnResourcesInNamespace(options.OtherTenantClient, options.TenantNamespace, resources, verbs)
		if err != nil {
			return err
		}

		err = utils.CheckAccessOnResourcesInNamespace(options.TenantClient, options.OtherNamespace, resources, verbs)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("block_other_tenant_resources/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b);
}
	