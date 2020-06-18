package benchmark

import (
	"errors"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"
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
	PreRun        func(string, *kubernetes.Clientset, *kubernetes.Clientset) error
	Run           func(string, *kubernetes.Clientset, *kubernetes.Clientset) error
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
