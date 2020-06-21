package test

import (
	suite "sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark_suite"
)

var BenchmarkSuite = &suite.BenchmarkSuite{
	Version: "1.0.0",
	Title:   "Multi-Tenancy Benchmarks",
}
