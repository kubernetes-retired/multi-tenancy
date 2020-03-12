package manager

import (
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type VirtualclusterManager struct {
	manager.Manager
	MaxConcurrentReconciles int
}

func NewVirtualClusterManager(config *rest.Config, options manager.Options, maxConcur int) (*VirtualclusterManager, error) {
	mgr, err := manager.New(config, options)
	if err != nil {
		return nil, err
	}
	return &VirtualclusterManager{
		Manager:                 mgr,
		MaxConcurrentReconciles: maxConcur,
	}, nil
}
