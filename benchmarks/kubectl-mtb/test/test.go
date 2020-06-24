package test

import (
	suite "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark_suite"
	blockprivilegedcontainers "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/benchmarks/block_privileged_containers"
)

var BenchmarkSuite = &suite.BenchmarkSuite{
	Version: "1.0.0",
	Title:   "Multi-Tenancy Benchmarks",
}

// NewBenchmarkSuite returns the pointer of benchmarksuite having collection of bechmarks
func NewBenchmarkSuite() *suite.BenchmarkSuite {

	// Add Benchmarks
	BenchmarkSuite.Add(blockprivilegedcontainers.NewBenchmark())

	return BenchmarkSuite
}
