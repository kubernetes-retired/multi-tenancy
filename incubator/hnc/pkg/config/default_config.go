package config

import (
	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

// GetDefaultConfigSpec creates the default configuration for HNCConfiguration Spec.
// By default, HNC configuration should always propagate Roles and RoleBindings.
// See details in http://bit.ly/hnc-type-configuration
func GetDefaultConfigSpec() api.HNCConfigurationSpec {
	return api.HNCConfigurationSpec{
		Types: []api.TypeSynchronizationSpec{
			{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: api.Propagate},
			{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: api.Propagate}},
	}
}
