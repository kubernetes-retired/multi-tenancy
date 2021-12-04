package requirealwayspullimage

import (
	"context"
	"fmt"
	"strings"

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
		pluginEnabled, err := isAlwaysPullImagesAdmissionPluginEnabled(options)
		if err != nil {
			options.Logger.Debug(err.Error())
			return err
		}
		if pluginEnabled {
			options.Logger.Debug("Skipping pod creation check since AlwaysPullImages is enabled")
			return nil
		}

		// ImagePullPolicy set to "Never" so that pod creation would fail
		podSpec := &podutil.PodSpec{NS: options.TenantNamespace, ImagePullPolicy: "Never", RunAsNonRoot: true}
		err = podSpec.SetDefaults()
		if err != nil {
			options.Logger.Debug(err.Error())
			return err
		}

		// Try to create a pod as tenant-admin impersonation
		pod := podSpec.MakeSecPod()
		_, err = options.Tenant1Client.CoreV1().Pods(options.TenantNamespace).Create(context.TODO(), pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err == nil {
			return fmt.Errorf("Tenant must be unable to create pod if ImagePullPolicy is not set to Always")
		}
		options.Logger.Debug("Test passed: ", err.Error())

		return nil
	},
}

func isAlwaysPullImagesAdmissionPluginEnabled(options types.RunOptions) (bool, error) {
	selector := "component=kube-apiserver"
	pods, err := options.ClusterAdminClient.CoreV1().Pods("kube-system").List(context.TODO(), metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return false, err
	}
	if len(pods.Items) == 0 {
		return false, fmt.Errorf("could not find a kube-apiserver pod with the label %s; could not determine if AlwaysPullImages is enabled", selector)
	}
	for _, line := range pods.Items[0].Spec.Containers[0].Command {
		if strings.HasPrefix(line, "--enable-admission-plugins") && strings.Contains(line, "AlwaysPullImages") {
			return true, nil
		}
	}
	return false, nil
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("require_always_pull_image/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
