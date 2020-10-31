package benchmark

import (
	"errors"

	"gopkg.in/yaml.v2"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"
)

// Benchmark consists the benchmark information like benchmark id, name, remediation etc.
type Benchmark struct {
	ID            string `yaml:"id"`
	Title         string `yaml:"title"`
	BenchmarkType string `yaml:"benchmarkType"`
	Category      string `yaml:"category"`
	Description   string `yaml:"description"`
	Remediation   string `yaml:"remediation"`
	ProfileLevel  int    `yaml:"profileLevel"`
	Status        string `yaml:"status"`
	Rationale     string `yaml:"rationale"`
	Audit         string `yaml:"audit"`
	NamespaceRequired int `yaml:"namespaceRequired"`
	PreRun        func(types.RunOptions) error
	Run           func(types.RunOptions) error
	PostRun       func(types.RunOptions) error
}

// ReadConfig reads the yaml representation of struct from []file
func (b *Benchmark) ReadConfig(file []byte) error {
	if err := yaml.Unmarshal(file, b); err != nil {
		return err
	}

	if b == nil {
		return errors.New("Please fill in a valid/non-empty yaml file")
	}

	return nil
}
