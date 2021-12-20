package requirealwayspullimage

import (
	"context"
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
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

		access, msg, err := utils.RunAccessCheck(options.Tenant1Client, options.TenantNamespace, resource, "create")
		if err != nil {
			options.Logger.Debug(err.Error())
			return err
		}
		if !access {
			return fmt.Errorf(msg)
		}

		return nil
	},
	Run: func(options types.RunOptions) error {
		// ImagePullPolicy set to "Never" so that pod creation would fail
		podSpec := &podutil.PodSpec{NS: options.TenantNamespace, ImagePullPolicy: "Never", RunAsNonRoot: true}
		err := podSpec.SetDefaults()
		if err != nil {
			options.Logger.Debug(err.Error())
			return err
		}

		// Try to create a pod as tenant-admin impersonation
		pod := podSpec.MakeSecPod()
		returnedPod, err := options.Tenant1Client.CoreV1().Pods(options.TenantNamespace).Create(context.TODO(), pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err != nil {
			// `admission webhook "validation.gatekeeper.sh" denied the request: ...`
			if strings.Contains(err.Error(), "admission webhook") {
				options.Logger.Debug("Test passed: ", err.Error())
				return nil
			}
			return err
		}
		// The pod was allowed, but was the returnedPod modified by AlwaysPullImages?
		if returnedPod.Spec.Containers[0].ImagePullPolicy != v1.PullAlways {
			return fmt.Errorf("tenant must be unable to create pods with ImagePullPolicy set to anything other than 'Always'")
		}
		options.Logger.Debug("Test passed: The pod's imagePullPolicy was mutated to 'Always'")
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("require_always_pull_image/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
