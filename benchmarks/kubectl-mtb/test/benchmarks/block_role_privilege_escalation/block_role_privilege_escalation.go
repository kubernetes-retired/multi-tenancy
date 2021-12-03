package blockroleprivilegeescalation

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/bundle/box"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"
)

var b = &benchmark.Benchmark{

	PreRun: func(options types.RunOptions) error {

		return nil
	},
	Run: func(options types.RunOptions) error {
		// Tenants shouldn't be allowed to "escalate" roles or "bind" to a higher-privileged role like cluster-admin.
		// Ref: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#privilege-escalation-prevention-and-bootstrapping
		roleResource := utils.GroupResource{
			APIGroup: "rbac.authorization.k8s.io",
			APIResource: metav1.APIResource{
				Name: "role",
			},
		}
		access, msg, err := utils.RunAccessCheck(options.Tenant1Client, options.TenantNamespace, roleResource, "escalate")
		if err != nil {
			options.Logger.Debug(err.Error())
			return err
		}
		if access {
			return fmt.Errorf(msg)
		}
		// This mainly checks if tenants have been _accidentally_ given access to bind to any role, via "resourceNames: ['*']".
		// There's still the chance they have access to bind to specific roles, but that would likely have been set explicitly by an admin.
		crResource := utils.GroupResource{
			APIGroup: "rbac.authorization.k8s.io",
			APIResource: metav1.APIResource{
				Name: "clusterrole",
			},
			ResourceName: "cluster-admin",
		}
		access, msg, err = utils.RunAccessCheck(options.Tenant1Client, options.TenantNamespace, crResource, "bind")
		if err != nil {
			options.Logger.Debug(err.Error())
			return err
		}
		if access {
			return fmt.Errorf(msg)
		}
		return nil
	},
}

func init() {
	// Get the []byte representation of a file, or an error if it doesn't exist:
	err := b.ReadConfig(box.Get("block_role_privilege_escalation/config.yaml"))
	if err != nil {
		fmt.Println(err)
	}

	test.BenchmarkSuite.Add(b)
}
