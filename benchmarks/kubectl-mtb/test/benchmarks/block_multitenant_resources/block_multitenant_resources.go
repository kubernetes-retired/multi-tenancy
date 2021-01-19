package blockmultitenantresources

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"
)

type gvr struct {
	APIVersion  string
	APIGroup    string
	APIResource string
}

var resources []gvr

var b = &benchmark.Benchmark{

	PreRun: func(options types.RunOptions) error {

		if options.Label == "" {
			return fmt.Errorf("Label must be set via -l or --label")
		}

		return nil
	},
	Run: func(options types.RunOptions) error {

		lists, err := options.ClusterAdminClient.Discovery().ServerPreferredResources()
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

				if !resource.Namespaced {
					continue
				}
				resources = append(resources, gvr{
					APIGroup:    gv.Group,
					APIResource: resource.Name,
					APIVersion:  gv.Version,
				})
			}
		}

		for _, resource := range resources {

			gvr := schema.GroupVersionResource{
				Group:    resource.APIGroup,
				Version:  resource.APIVersion,
				Resource: resource.APIResource,
			}
			kubecfgFlags := genericclioptions.NewConfigFlags(false)

			config, err := kubecfgFlags.ToRESTConfig()
			if err != nil {
				fmt.Println(err.Error())
				return err
			}
			config.Impersonate.UserName = options.Tenant
			dynClient, errClient := dynamic.NewForConfig(config)
			if errClient != nil {
				fmt.Println(errClient.Error())
				return errClient
			}

			client := dynClient.Resource(gvr)

			labelArray := strings.Split(options.Label, "=")
			labelMap := map[string]string{labelArray[0]: labelArray[1]}
			opts := metav1.ListOptions{
				LabelSelector: labels.SelectorFromSet(labelMap).String(),
			}
			resourceList, err := client.Namespace(options.TenantNamespace).List(context.TODO(), opts)
			if err != nil {
				// fmt.Println(errC.Error())
			} else {
				for _, resourceObj := range resourceList.Items {
					err := client.Namespace(options.TenantNamespace).Delete(context.TODO(), resourceObj.GetName(), metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}})
					if err == nil {
						return fmt.Errorf("Tenant can delete %v %v", gvr.Resource, resourceObj.GetName())
					}
				}
			}
		}

		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("block_multitenant_resources/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
