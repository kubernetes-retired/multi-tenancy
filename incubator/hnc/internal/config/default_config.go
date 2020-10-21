package config

import (
	api "sigs.k8s.io/multi-tenancy/incubator/hnc/api/v1alpha2"
)

// The EX map is used by reconcilers and validators to exclude
// namespaces that shouldn't be reconciled or validated. We explicitly
// exclude some default namespaces with constantly changing objects.
// TODO make the exclusion configurable - https://github.com/kubernetes-sigs/multi-tenancy/issues/374
var EX = map[string]bool{
	"kube-system":  true,
	"kube-public":  true,
	"hnc-system":   true,
	"cert-manager": true,
}

// GetDefaultRoleSpec and GetDefaultRoleBindingSpec create the default
// configuration for Roles and RoleBindings respectively.
// By default, HNC configuration should always propagate Roles and RoleBindings.
// See details in http://bit.ly/hnc-type-configuration

func GetDefaultRoleSpec() api.ResourceSpec {
	return api.ResourceSpec{Group: api.RBACGroup, Resource: api.RoleResource, Mode: api.Propagate}
}

func GetDefaultRoleBindingSpec() api.ResourceSpec {
	return api.ResourceSpec{Group: api.RBACGroup, Resource: api.RoleBindingResource, Mode: api.Propagate}
}
