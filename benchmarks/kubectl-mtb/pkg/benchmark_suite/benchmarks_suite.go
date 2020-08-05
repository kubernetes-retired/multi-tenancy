package benchmarksuite

import (
	"sort"
	"strconv"
	"strings"

	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
)

// BenchmarkSuite - Collection of benchmarks
type BenchmarkSuite struct {
	Version    string
	Title      string
	Benchmarks []*benchmark.Benchmark
}

// SortedBenchmarks contains benchmarks sorted according to profile level, category and id
var SortedBenchmarks []*benchmark.Benchmark

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
	SortedBenchmarks = sortBenchmarks(benchmarksArray)
	return SortedBenchmarks
}

// sortBenchmarks returns slice of Benchmarks sorted according to Profile level, category and id respectively
func sortBenchmarks(benchmarks []*benchmark.Benchmark) []*benchmark.Benchmark {
	sort.SliceStable(benchmarks, func(i, j int) bool {
		b1, b2 := benchmarks[i], benchmarks[j]
		switch {
		//sort according to Profile Level
		case b1.ProfileLevel != b2.ProfileLevel:
			return b1.ProfileLevel < b2.ProfileLevel
		// sort according to category (lexicographical order)
		case returnCategory(b1.ID) != returnCategory(b2.ID):
			return returnCategory(b1.ID) < returnCategory(b2.ID)
		// sort according to sequence
		default:
			return returnID(b1.ID) < returnID(b2.ID)
		}
	})
	return benchmarks
}

// returns category by parsing id
func returnCategory(id string) string {
	result := strings.Split(id, "-")
	return result[len(result)-2]
}

// returns sequence by parsing id
func returnID(id string) int {
	result := strings.Split(id, "-")
	res, _ := strconv.Atoi(result[len(result)-1])
	return res
}
