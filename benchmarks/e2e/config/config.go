package test

import (
	"reflect"
)

const ConfigPath = "../../config.yaml"

type BenchmarkConfig struct {
	Adminkubeconfig string     `json:"adminKubeconfig"`
	Label           string     `json:"label,omitempty"`
	TenantA         TenantSpec `json:"tenantA,omitempty"`
	TenantB         TenantSpec `json:"tenantB,omitempty"`
}

type TenantSpec struct {
	Kubeconfig string `json:"kubeconfig"`
	Namespace  string `json:"namespace"`
}

func (c *BenchmarkConfig) GetValidTenant() TenantSpec {
	if !reflect.DeepEqual(c.TenantA, TenantSpec{}) {
		return c.TenantA
	}

	return c.TenantB
}
