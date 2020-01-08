package virtualcluster

import (
	tenancyv1alpha1 "github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/pkg/apis/tenancy/v1alpha1"
)

type MasterProvisioner interface {
	CreateVirtualCluster(vc *tenancyv1alpha1.Virtualcluster) error
	DeleteVirtualCluster(vc *tenancyv1alpha1.Virtualcluster) error
}
