package benchmarksuite

import (
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
)

// BenchmarkSuite - Collection of benchmarks
type BenchmarkSuite struct {
	Version    string
	Title      string
	Benchmarks []*benchmark.Benchmark
}

// GetTotalBenchmarks returns count of benchmarks in Benchmark Suite
func (bs *BenchmarkSuite) GetTotalBenchmarks() int {
	return len(bs.Benchmarks)
}

// AddBenchmark appends the benchmark in the BenchmarkSuite
func (bs *BenchmarkSuite) AddBenchmark(benchmark *benchmark.Benchmark) {
	bs.Benchmarks = append(bs.Benchmarks, benchmark)
}

// GetBenchmarksOfProfileLevel return slice of Benchmarks of Profile level given in input
func (bs *BenchmarkSuite) GetBenchmarksOfProfileLevel(pl int) []*benchmark.Benchmark {
	benchmarksArray := []*benchmark.Benchmark{}
	for _, b := range bs.Benchmarks {
		if b.ProfileLevel <= pl {
			benchmarksArray = append(benchmarksArray, b)
		}
	}
	return benchmarksArray
}
