package config

import "k8s.io/apimachinery/pkg/runtime/schema"

// GVKs is currently hardcoded to the set of GVKs handled by the HNC - namely, Secrets,
// Roles and RoleBindings - but in the future we should get this from a
// configuration object.
var GVKs = []schema.GroupVersionKind{
	{Group: "", Version: "v1", Kind: "Secret"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "Role"},
	{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "RoleBinding"},
	{Group: "networking.k8s.io", Version: "v1", Kind: "NetworkPolicy"},
	{Group: "", Version: "v1", Kind: "ResourceQuota"},
	{Group: "", Version: "v1", Kind: "LimitRange"},
	{Group: "", Version: "v1", Kind: "ConfigMap"},
}
