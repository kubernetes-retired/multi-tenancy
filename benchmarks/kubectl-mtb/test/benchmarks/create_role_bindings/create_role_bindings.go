package createrolebindings

import (
	"context"
	"fmt"

	v1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
)

var b = &benchmark.Benchmark{

	PreRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		return nil
	},
	Run: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {

		// Check for rbac privileges of role and rolebinding
		verbs := []string{"get", "list", "create", "update", "patch", "watch", "delete", "deletecollection"}
		resources := []utils.GroupResource{
			{
				APIGroup: "rbac.authorization.k8s.io",
				APIResource: metav1.APIResource{
					Name: "roles",
				},
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				APIResource: metav1.APIResource{
					Name: "rolebindings",
				},
			},
		}

		for _, resource := range resources {
			for _, verb := range verbs {
				access, msg, err := utils.RunAccessCheck(tclient, tenantNamespace, resource, verb)
				if err != nil {
					return err
				}
				if !access {
					return fmt.Errorf(msg)
				}
			}
		}

		// Trying to create a role and rolebinding for the same
		role := &v1.Role{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Role",
				APIVersion: "rbac.authorization.k8s.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: "role-sample"},
			Rules: []v1.PolicyRule{
				{
					Verbs:           []string{"get"},
					APIGroups:       []string{""},
					Resources:       []string{"pods"},
					ResourceNames:   nil,
					NonResourceURLs: nil,
				},
			},
		}

		role, err := tclient.RbacV1().Roles(tenantNamespace).Create(context.TODO(), role, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		roleref := v1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     role.ObjectMeta.Name,
		}

		roleBinding := &v1.RoleBinding{
			TypeMeta: metav1.TypeMeta{
				Kind:       "RoleBinding",
				APIVersion: "rbac.authorization.k8s.io/v1",
			},
			ObjectMeta: metav1.ObjectMeta{Name: "rolebinding-sample"},
			RoleRef:    roleref,
		}

		_, err = tclient.RbacV1().RoleBindings(tenantNamespace).Create(context.TODO(), roleBinding, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		if err != nil {
			return err
		}

		return nil
	},

	PostRun: func(tenantNamespace string, kclient, tclient *kubernetes.Clientset) error {
		err := tclient.RbacV1().Roles(tenantNamespace).Delete(context.TODO(), "role-sample", metav1.DeleteOptions{})
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("create_role_bindings/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
