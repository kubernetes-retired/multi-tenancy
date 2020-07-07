package blockaccesstoclusterresources

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/util"
)

var verbs = []string{"get", "update"}

var b = &benchmark.Benchmark{

	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		return nil
	},
	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {
		resources := []util.GroupResource{}

		lists, err := kclient.Discovery().ServerPreferredResources()
		if err != nil {
			return nil
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

				if resource.Namespaced {
					continue
				}
				resources = append(resources, util.GroupResource{
					APIGroup:    gv.Group,
					APIResource: resource,
				})
			}
		}

		for _, resource := range resources {
			for _, verb := range verbs {
				access, msg, err := util.RunAccessCheck(tclient, tenantNamespace, resource, verb)
				if err != nil {
					return err
				}
				if access {
					return fmt.Errorf(msg)
				}
			}
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("block_access_to_cluster_resources/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b);
}
	