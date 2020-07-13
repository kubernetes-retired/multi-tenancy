package configurensquotas

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
)

func getResourceNameList(tclient *kubernetes.Clientset) ([]string, error) {
	nodes, err := tclient.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return []string{}, err
	}
	return getResourcequotaFromNodes(*nodes), nil
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

var b = &benchmark.Benchmark{
	// Check if user can list nodes
	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {
		resources := []utils.GroupResource{
			{
				APIGroup: "",
				APIResource: metav1.APIResource{
					Name: "nodes",
				},
			},
		}
		verb := "list"
		for _, resource := range resources {
			access, msg, err := utils.RunAccessCheck(tclient, tenantNamespace, resource, verb)
			if err != nil {
				fmt.Println(err.Error())
			}
			if !access {
				return fmt.Errorf(msg)
			}
		}
		return nil
	},
	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		resourceNameList, err := getResourceNameList(tclient)
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
		tenantResourcequotas := utils.GetTenantResoureQuotas(tenantNamespace, tclient)
		expectedVal := strings.Join(tenantResourcequotas, " ")
		for _, r := range resourceNameList {
			if !strings.Contains(expectedVal, r) {
				fmt.Println(expectedVal)
				fmt.Println(r)
				return fmt.Errorf("%s must be configured in tenant %s namespace resource quotas", r, tenantNamespace)
			}
		}

		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("configure_ns_quotas/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
