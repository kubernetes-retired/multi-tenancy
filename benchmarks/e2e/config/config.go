package test

import (
	"reflect"
)

const ConfigPath = "../../config.yaml"

type BenchmarkConfig struct {
	Adminkubeconfig string     `yaml:"adminKubeconfig"`
	Label           string     `yaml:"label,omitempty"`
	TenantA         TenantSpec `yaml:"tenantA,omitempty"`
	TenantB         TenantSpec `yaml:"tenantB,omitempty"`
}

type TenantSpec struct {
	Kubeconfig string `yaml:"kubeconfig"`
	Namespace  string `yaml:"namespace"`
}

func (c *BenchmarkConfig) GetValidTenant() TenantSpec {
	if !reflect.DeepEqual(c.TenantA, TenantSpec{}) {
		return c.TenantA
	}

	return c.TenantB
}
