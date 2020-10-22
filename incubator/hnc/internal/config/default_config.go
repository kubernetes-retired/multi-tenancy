package config

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
