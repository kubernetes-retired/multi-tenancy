package test

import (
	"errors"
	"reflect"
)

// ConfigFlagType is the type for flags for the tests
var ConfigPath string

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

func (c *BenchmarkConfig) GetValidTenant() (TenantSpec, error) {
	if c == nil {
		return TenantSpec{}, errors.New("Please fill in a valid/non-empty config.yaml")
	}
	if !reflect.DeepEqual(c.TenantA, TenantSpec{}) {
		return c.TenantA, nil
	}

	return c.TenantB, nil
}

func (c *BenchmarkConfig) ValidateTenant(t TenantSpec) (error) {
	if c == nil {
		return errors.New("Please fill in a valid/non-empty config.yaml")
	}

	if !reflect.DeepEqual(t, TenantSpec{}) {
		return nil
	}

	return errors.New("Given tenant does not match with TenantSpec")
} 
