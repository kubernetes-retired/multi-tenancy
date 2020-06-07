package benchmark

import (
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
	Run           func(string, *kubernetes.Clientset, *kubernetes.Clientset) (bool, error)
}
