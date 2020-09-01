package blockuseofhostnetworkingandports

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/log"

	podutil "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/resources/pod"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"
)

var b = &benchmark.Benchmark{

	PreRun: func(options types.RunOptions) error {

		resource := utils.GroupResource{
			APIGroup: "",
			APIResource: metav1.APIResource{
				Name: "pods",
			},
		}

		access, msg, err := utils.RunAccessCheck(options.TClient, options.TenantNamespace, resource, "create")
		if err != nil {
			log.Logging.Debug(err.Error())
			return err
		}
		if !access {
			return fmt.Errorf(msg)
		}

		return nil
	},

	Run: func(options types.RunOptions) error {

		//Tenant containers cannot use host networking
		podSpec := &podutil.PodSpec{NS: options.TenantNamespace, HostNetwork: true, Ports: nil, RunAsNonRoot: false}
		err := podSpec.SetDefaults()
		if err != nil {
			log.Logging.Debug(err.Error())
			return err
		}

		// Try to create a pod as tenant-admin impersonation
		pod := podSpec.MakeSecPod()
		_, err = options.TClient.CoreV1().Pods(options.TenantNamespace).Create(context.TODO(), pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err == nil {
			return fmt.Errorf("Tenant must be unable to create pod with host networking set to true")
		}

		//Tenant should not be allowed to use host ports
		ports := []v1.ContainerPort{
			{
				HostPort:      8086,
				ContainerPort: 8086,
			},
		}

		podSpec1 := &podutil.PodSpec{NS: options.TenantNamespace, HostNetwork: false, Ports: ports, RunAsNonRoot: true}
		err = podSpec.SetDefaults()
		if err != nil {
			log.Logging.Debug(err.Error())
			return err
		}

		// Try to create a pod as tenant-admin impersonation
		pod1 := podSpec1.MakeSecPod()
		_, err = options.TClient.CoreV1().Pods(options.TenantNamespace).Create(context.TODO(), pod1, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err == nil {
			return fmt.Errorf("Tenant must be unable to create pod with defined host ports")
		}
		log.Logging.Debug("Test Passed: ", err.Error())
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("block_use_of_host_networking_and_ports/config.yaml"))
	if err != nil {
		log.Logging.Error(err.Error())
	}

	test.BenchmarkSuite.Add(b)
}
