package config

// UnpropgatedAnnotations is a list of annotations on objects that should _not_ be propagated by HNC.
// Much like HNC itself, other systems (such as GKE Config Sync) use annotations to "claim" an
// object - such as deleting objects it doesn't recognize. By removing these annotations on
// propgated objects, HNC ensures that other systems won't attempt to claim the same object.
//
// This value is controlled by the --unpropagated-annotation command line, which may be set multiple
// times.
var UnpropagatedAnnotations []string

// ExcludedNamespaces is a list of namespaces used by reconcilers and validators
// to exclude namespaces that shouldn't be reconciled or validated.
//
// This value is controlled by the --excluded-namespace command line, which may
// be set multiple times.
var ExcludedNamespaces map[string]bool
