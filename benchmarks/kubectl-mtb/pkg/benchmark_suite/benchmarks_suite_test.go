package benchmarksuite

import (
	"reflect"
	"testing"

	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
)

var benchmarkArray = []*benchmark.Benchmark{
	&benchmark.Benchmark{
		ID:           "MTB-1",
		ProfileLevel: 1,
		Category:     "control plane Isolation",
	},
	&benchmark.Benchmark{
		ID:           "MTB-2",
		ProfileLevel: 2,
		Category:     "host ISOLATION",
	},
	&benchmark.Benchmark{
		ID:           "MTB-3",
		Category:     "host ProTecTion",
		ProfileLevel: 3,
	},
}

func TestAddBenchmark(t *testing.T) {
	bs := &BenchmarkSuite{Title: "MTB", Version: "1.0.0"}

	for _, b := range benchmarkArray {
		bs.AddBenchmark(b)
	}

	if !reflect.DeepEqual(benchmarkArray, bs.Benchmarks) {
		t.Errorf("Error in adding benchmark to benchmark suite.")
	}
}

func TestGetTotalBenchmarks(t *testing.T) {
	bs := &BenchmarkSuite{Title: "MTB", Version: "1.0.0"}
	for _, b := range benchmarkArray {
		bs.AddBenchmark(b)
	}
	if bs.GetTotalBenchmarks() != len(benchmarkArray) {
		t.Errorf("Error in adding benchmark to benchmark suite.")
	}
}

func TestGetBenchmarksOfProfileLevel(t *testing.T) {
	tests := []struct {
		benchmarks   []*benchmark.Benchmark
		profileLevel int
	}{
		{
			benchmarks: []*benchmark.Benchmark{
				&benchmark.Benchmark{
					ID:           "MTB-1",
					ProfileLevel: 1,
					Category:     "control plane Isolation",
				},
			},
			profileLevel: 1,
		},
		{
			benchmarks: []*benchmark.Benchmark{
				&benchmark.Benchmark{
					ID:           "MTB-1",
					ProfileLevel: 1,
					Category:     "control plane Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-2",
					ProfileLevel: 2,
					Category:     "host ISOLATION",
				},
			},
			profileLevel: 2,
		},
		{
			benchmarks: []*benchmark.Benchmark{
				&benchmark.Benchmark{
					ID:           "MTB-1",
					ProfileLevel: 1,
					Category:     "control plane Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-2",
					ProfileLevel: 2,
					Category:     "host ISOLATION",
				},
				&benchmark.Benchmark{
					ID:           "MTB-3",
					Category:     "host ProTecTion",
					ProfileLevel: 3,
				},
			},
			profileLevel: 3,
		},
	}

	bs := &BenchmarkSuite{Title: "MTB", Version: "1.0.0"}
	for _, b := range benchmarkArray {
		bs.AddBenchmark(b)
	}
	for _, item := range tests {
		filteredBenchmarks := bs.GetBenchmarksOfProfileLevel(item.profileLevel)
		if !reflect.DeepEqual(item.benchmarks, filteredBenchmarks) {
			t.Errorf("Error in filtering the benchmarks according to Profile Level of %d.", item.profileLevel)
		}
	}
}
