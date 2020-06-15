package blockprivilegedcontainers

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	podutil "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/util/resources/pod"
)

var bpcBenchmark = &benchmark.Benchmark{
	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) (bool, error) {

		// IsPrivileged set to true so that pod creation would fail
		podSpec := &podutil.PodSpec{NS: tenantNamespace, IsPrivileged: true}
		err := podSpec.SetDefaults()
		if err != nil {
			return false, err
		}

		// Try to create a pod as tenant-admin impersonation
		pod := podSpec.MakeSecPod()
		_, err = tclient.CoreV1().Pods(tenantNamespace).Create(context.TODO(), pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err == nil {
			return false, fmt.Errorf("Tenant must be unable to create pod that sets privileged to true")
		}
		fmt.Println(err.Error())
		return true, nil
	},
}

// NewBenchmark returns the pointer of the benchmark
func NewBenchmark() *benchmark.Benchmark {

	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := bpcBenchmark.ReadConfig(box.Get("block_privileged_containers/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	return bpcBenchmark
}
