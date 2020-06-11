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

// Totals returns count of benchmarks in Benchmark Suite
func (bs *BenchmarkSuite) Totals() int {
	return len(bs.Benchmarks)
}

// Add appends the benchmark in the BenchmarkSuite
func (bs *BenchmarkSuite) Add(benchmark *benchmark.Benchmark) {
	bs.Benchmarks = append(bs.Benchmarks, benchmark)
}

// ProfileLevel return slice of Benchmarks of Profile level given in input
func (bs *BenchmarkSuite) ProfileLevel(pl int) []*benchmark.Benchmark {
	benchmarksArray := []*benchmark.Benchmark{}
	for _, b := range bs.Benchmarks {
		if b.ProfileLevel <= pl {
			benchmarksArray = append(benchmarksArray, b)
		}
	}
	return benchmarksArray
}
