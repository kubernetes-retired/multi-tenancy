package types

import (
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
)

// RunOptions contains benchmark
type RunOptions struct {
	Tenant          string
	TenantNamespace string
	Cmd             *cobra.Command
	Args            []string
	KClient         *kubernetes.Clientset
	TClient         *kubernetes.Clientset
}
