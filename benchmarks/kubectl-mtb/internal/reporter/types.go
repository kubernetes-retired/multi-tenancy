package reporter

import (
	"fmt"
	"time"

	"github.com/creasty/defaults"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	benchmarksuite "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark_suite"
)

// SuiteSummary summaries the result of benchmark suite
type SuiteSummary struct {
	Namespace                 string
	User                      string
	NumberOfTotalTests        int
	NumberOfPassedTests       int
	NumberOfFailedTests       int
	NumberOfSkippedTests      int
	NumberOfFailedValidations int
	RunTime                   time.Duration
	Suite                     *benchmarksuite.BenchmarkSuite
}

// TestSummary summaries the result of benchmark
type TestSummary struct {
	Validation      bool `default:"true"`
	ValidationError error
	Test            bool `default:"true"`
	TestError       error
	RunTime         time.Duration
	Benchmark       *benchmark.Benchmark
}

// SetDefaults usage := https://github.com/creasty/defaults#usage
func (t *TestSummary) SetDefaults() error {
	if err := defaults.Set(t); err != nil {
		return fmt.Errorf("it should not return an error: %v", err)
	}
	return nil
}
