package blockuseofnodeportservices

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes"
	e2edeployment "k8s.io/kubernetes/test/e2e/framework/deployment"
	imageutils "k8s.io/kubernetes/test/utils/image"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
	serviceutil "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/resources/service"
)

var b = &benchmark.Benchmark{

	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		resources := []utils.GroupResource {
			{
				APIGroup: "",
				APIResource: metav1.APIResource{
					Name: "services",
				},
			},
			{
				APIGroup: "apps",
				APIResource: metav1.APIResource{
					Name: "deployments",
				},
			},
		}

		for _, resource := range resources {
			access, msg, err := utils.RunAccessCheck(tclient, tenantNamespace, resource, "create")
			if err != nil {
				return err
			}
			if !access {
				return fmt.Errorf(msg)
			}
		}

		return nil
	},

	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		podLabels := map[string]string{"test": "multi"}
		deploymentName := "deployment-" + string(uuid.NewUUID())
		imageName := "image-" + string(uuid.NewUUID())
		deployment := e2edeployment.NewDeployment(deploymentName, 1, podLabels, imageName, imageutils.GetE2EImage(imageutils.Nginx), "Recreate")

		_, err := tclient.AppsV1().Deployments(tenantNamespace).Create(context.TODO(), deployment, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err != nil {
			return err
		}

		svcSpec := &serviceutil.ServiceConfig{Type: "NodePort", Selector: podLabels}
		svc := svcSpec.CreateServiceSpec()
		_, err = tclient.CoreV1().Services(tenantNamespace).Create(context.TODO(), svc, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})

		if err == nil {
			return fmt.Errorf("Tenant must be unable to create service of type NodePort")
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("block_use_of_nodeport_services/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
