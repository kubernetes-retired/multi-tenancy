package blockuseofbindmounts

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/util"
	podutil "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/util/resources/pod"
)

var b = &benchmark.Benchmark{

	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		resource := util.GroupResource{
			APIGroup: "",
			APIResource: metav1.APIResource{
				Name: "pods",
			},
		}

		access, msg, err := util.RunAccessCheck(tclient, tenantNamespace, resource, "create")
		if err != nil {
			return err
		}
		if !access {
			return fmt.Errorf(msg)
		}

		return nil
	},

	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		// Host path
		inlineVolumeSources := []*v1.VolumeSource{
			{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/tmp/busybox",
				},
			},
		}

		podSpec := &podutil.PodSpec{NS: tenantNamespace, InlineVolumeSources: inlineVolumeSources}
		err := podSpec.SetDefaults()
		if err != nil {
			return err
		}

		// Try to create a pod as tenant-admin impersonation
		pod := podSpec.MakeSecPod()
		_, err = tclient.CoreV1().Pods(tenantNamespace).Create(context.TODO(), pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err == nil {
			return fmt.Errorf("Tenant must be unable to create pod with host-path volume")
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("block_use_of_bind_mounts/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
