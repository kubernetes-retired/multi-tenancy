package config

// EX is a map used by reconcilers and validators to exclude namespaces that shouldn't be reconciled
// or validated. We explicitly exclude some default namespaces with constantly changing objects.
//
// TODO make the exclusion configurable -
// https://github.com/kubernetes-sigs/multi-tenancy/issues/374
var EX = map[string]bool{
	"kube-system":     true,
	"kube-public":     true,
	"hnc-system":      true,
	"cert-manager":    true,
	"kube-node-lease": true,
}

// UnpropgatedAnnotations is a list of annotations on objects that should _not_ be propagated by HNC.
// Much like HNC itself, other systems (such as GKE Config Sync) use annotations to "claim" an
// object - such as deleting objects it doesn't recognize. By removing these annotations on
// propgated objects, HNC ensures that other systems won't attempt to claim the same object.
//
// This value is controlled by the --unpropagated-annotation command line, which may be set multiple
// times.
var UnpropagatedAnnotations []string
