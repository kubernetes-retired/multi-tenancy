package config

import (
	api "github.com/kubernetes-sigs/multi-tenancy/incubator/hnc/api/v1alpha1"
)

// GetDefaultRoleSpec and GetDefaultRoleBindingSpec create the default
// configuration for Roles and RoleBindings respectively.
// By default, HNC configuration should always propagate Roles and RoleBindings.
// See details in http://bit.ly/hnc-type-configuration

func GetDefaultRoleSpec() api.TypeSynchronizationSpec {
	return api.TypeSynchronizationSpec{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "Role", Mode: api.Propagate}
}

func GetDefaultRoleBindingSpec() api.TypeSynchronizationSpec {
	return api.TypeSynchronizationSpec{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "RoleBinding", Mode: api.Propagate}
}
