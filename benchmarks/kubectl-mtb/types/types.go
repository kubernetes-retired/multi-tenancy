package types

import (
	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
)

// RunOptions contains benchmark running options
type RunOptions struct {
	Tenant             string
	TenantNamespace    string
	OtherTenant        string
	OtherNamespace     string
	Label              string
	ClusterAdminClient *kubernetes.Clientset
	Tenant1Client      *kubernetes.Clientset
	Tenant2Client      *kubernetes.Clientset
	Logger             *zap.SugaredLogger
}
