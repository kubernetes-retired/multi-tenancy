package blockprivilegedcontainers

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	podutil "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/resources/pod"
)

type GroupResource struct {
	APIGroup    string
	APIResource metav1.APIResource
}

func RunAccessCheck(client *kubernetes.Clientset, namespace string, resource GroupResource, verb string) (bool, string, error) {
	var sar *authorizationv1.SelfSubjectAccessReview

	// Todo for non resource url
	sar = &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace:   namespace,
				Verb:        verb,
				Group:       resource.APIGroup,
				Resource:    resource.APIResource.Name,
				Subresource: "",
				Name:        "",
			},
		},
	}

	response, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(context.TODO(), sar, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
	if err != nil {
		return false, "", err
	}

	if response.Status.Allowed {
		return true, fmt.Sprintf("User can %s %s", verb, resource.APIResource.Name), nil
	}

	return false, fmt.Sprintf("User cannot %s %s", verb, resource.APIResource.Name), nil
}

var b = &benchmark.Benchmark{

	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		resource := GroupResource{
			APIGroup: "",
			APIResource: metav1.APIResource{
				Name: "pods",
			},
		}

		access, msg, err := RunAccessCheck(tclient, tenantNamespace, resource, "create")
		if err != nil {
			return err
		}
		if !access {
			return fmt.Errorf(msg)
		}

		return nil
	},
	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		// IsPrivileged set to true so that pod creation would fail
		podSpec := &podutil.PodSpec{NS: tenantNamespace, IsPrivileged: true}
		err := podSpec.SetDefaults()
		if err != nil {
			return err
		}

		// Try to create a pod as tenant-admin impersonation
		pod := podSpec.MakeSecPod()
		_, err = tclient.CoreV1().Pods(tenantNamespace).Create(context.TODO(), pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err == nil {
			return fmt.Errorf("Tenant must be unable to create pod that sets privileged to true")
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("block_privileged_containers/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
