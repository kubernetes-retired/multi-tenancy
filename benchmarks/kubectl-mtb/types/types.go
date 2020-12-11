package types

import (
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

// RunOptions contains benchmark running options
type RunOptions struct {
	Tenant          string
	TenantNamespace string
	OtherTenant 	string
	OtherNamespace 	string
	Label           string
	KClient         *kubernetes.Clientset
	TClient         *kubernetes.Clientset
	Logger          *zap.SugaredLogger
}
