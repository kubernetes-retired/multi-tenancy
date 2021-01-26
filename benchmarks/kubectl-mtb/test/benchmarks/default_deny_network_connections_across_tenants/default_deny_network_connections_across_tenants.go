package defaultdenynetworkconnectionsacrosstenants

import (
	"context"
	"fmt"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	podutil "k8s.io/kubernetes/test/e2e/framework/pod"
	"os"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"
	"time"

	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
)


func makeSecPod(imageName string, namespace string, podName string) *v1.Pod {
	var runAsNonRoot = false
	var runAsUser = int64(1000)
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels:    map[string]string{"run": "my-nginx"},
		},
		Spec: v1.PodSpec{
			SecurityContext: &v1.PodSecurityContext{
				RunAsNonRoot: &runAsNonRoot,
				RunAsUser: &runAsUser,
			},
			Containers: []v1.Container{
				{
					Name:  podName,
					ImagePullPolicy: "Always",
					Image: "nginxinc/nginx-unprivileged",
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							"cpu":    resource.MustParse("0m"),
							"memory": resource.MustParse("0Gi"),
						},
						Requests: v1.ResourceList{
							"cpu":    resource.MustParse("0m"),
							"memory": resource.MustParse("0Gi"),
						},
					},
					SecurityContext: &v1.SecurityContext{
						RunAsNonRoot: &runAsNonRoot,
						RunAsUser: &runAsUser,
					},
				},
			},
			RestartPolicy: v1.RestartPolicyAlways,
		},
	}
}

var b = &benchmark.Benchmark{

	PreRun: func(options types.RunOptions) error {

		resources := []utils.GroupResource{
			{
				APIGroup: "",
				APIResource: metav1.APIResource{
					Name: "services",
				},
			},
			{
				APIGroup: "",
				APIResource: metav1.APIResource{
					Name: "pods",
				},
			},
		}

		for _, resource := range resources {
			access, msg, err := utils.RunAccessCheck(options.Tenant1Client, options.TenantNamespace, resource, "create")
			if err != nil {
				options.Logger.Debug(err.Error())
				return err
			}
			if !access {
				return fmt.Errorf(msg)
			}
		}
		return nil
	},
	Run: func(options types.RunOptions) error {
		pod := makeSecPod("nginxinc/nginx-unprivileged", options.TenantNamespace, "nginx-test")
		_, err := options.Tenant1Client.CoreV1().Pods(options.TenantNamespace).Create(context.TODO(), pod, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		for {
			if podutil.CheckPodsRunningReady(options.ClusterAdminClient, options.TenantNamespace,[]string{"nginx-test"}, 400*time.Second) {
				break
			}
		}

		nginxPod, err := options.ClusterAdminClient.CoreV1().Pods(options.TenantNamespace).Get(context.TODO(), "nginx-test", metav1.GetOptions{})
		if err != nil {
			return err
		}

		nginxPodIp := nginxPod.Status.PodIP

		busyBoxPod := makeSecPod("joeshaw/busybox-nonroot", options.OtherNamespace, "busy-box")

		// Try to create a pod as tenant-admin impersonation
		_, err = options.Tenant2Client.CoreV1().Pods(options.OtherNamespace).Create(context.TODO(), busyBoxPod, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		for {
			if podutil.CheckPodsRunningReady(options.ClusterAdminClient, options.OtherNamespace,[]string{"busy-box"}, 400*time.Second) {
				break
			}
		}

		cmd := []string{"curl", "--connect-timeout",  "5", nginxPodIp + ":" + "8080"}
		req := options.ClusterAdminClient.CoreV1().RESTClient().Post().Resource("pods").Name("busy-box").
			Namespace(options.OtherNamespace).SubResource("exec")
		option := &v1.PodExecOptions{
			Command: cmd,
			Stdin:   true,
			Stdout:  true,
			Stderr:  true,
			TTY:     true,
		}
		req.VersionedParams(
			option,
			scheme.ParameterCodec,
		)
		kubecfgFlags := genericclioptions.NewConfigFlags(false)
		config, err := kubecfgFlags.ToRESTConfig()
		if err != nil {
			return err
		}
		exec, err := remotecommand.NewSPDYExecutor(config, "GET", req.URL())
		if err != nil {
			return err
		}
		err = exec.Stream(remotecommand.StreamOptions{
			Stdin: os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		})
		if err != nil {
			options.Logger.Debug(err.Error())
			return nil
		}

		err = options.ClusterAdminClient.CoreV1().Pods(options.OtherNamespace).Delete(context.TODO(), "busy-box", metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		err = options.ClusterAdminClient.CoreV1().Pods(options.TenantNamespace).Delete(context.TODO(), "nginx-test", metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		return fmt.Errorf("tenant should not allowed be allowed to connect resources of other namespace")
	},

	PostRun: func(options types.RunOptions) error {
		err := options.ClusterAdminClient.CoreV1().Pods(options.OtherNamespace).Delete(context.TODO(), "busy-box", metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		err = options.ClusterAdminClient.CoreV1().Pods(options.TenantNamespace).Delete(context.TODO(), "nginx-test", metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("default_deny_network_connections_across_tenants/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b);
}
