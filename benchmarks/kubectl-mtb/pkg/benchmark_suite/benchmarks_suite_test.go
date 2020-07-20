package benchmarksuite

import (
	"fmt"
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
		bs.Add(b)
	}

	if !reflect.DeepEqual(benchmarkArray, bs.Benchmarks) {
		t.Errorf("Error in adding benchmark to benchmark suite.")
	}
}

func TestGetTotalBenchmarks(t *testing.T) {
	bs := &BenchmarkSuite{Title: "MTB", Version: "1.0.0"}
	for _, b := range benchmarkArray {
		bs.Add(b)
	}
	if bs.Totals() != len(benchmarkArray) {
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
		bs.Add(b)
	}
	for _, item := range tests {
		filteredBenchmarks := bs.ProfileLevel(item.profileLevel)
		if !reflect.DeepEqual(item.benchmarks, filteredBenchmarks) {
			t.Errorf("Error in filtering the benchmarks according to Profile Level of %d.", item.profileLevel)
		}
	}
}

func TestSortBenchmarks(t *testing.T) {
	tests := []struct {
		testBenchmarks     []*benchmark.Benchmark
		expectedBenchmarks []*benchmark.Benchmark
	}{
		{
			testBenchmarks: []*benchmark.Benchmark{
				&benchmark.Benchmark{
					ID:           "MTB-PL2-CC-CPI-2",
					ProfileLevel: 2,
					Category:     "control plane Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL1-CC-HI-1",
					ProfileLevel: 1,
					Category:     "Host Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL1-CC-CPI-1",
					ProfileLevel: 1,
					Category:     "control plane Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL1-CC-CPI-2",
					ProfileLevel: 1,
					Category:     "control plane Isolation",
				},
			},

			expectedBenchmarks: []*benchmark.Benchmark{
				&benchmark.Benchmark{
					ID:           "MTB-PL1-CC-CPI-1",
					ProfileLevel: 1,
					Category:     "control plane Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL1-CC-CPI-2",
					ProfileLevel: 1,
					Category:     "control plane Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL1-CC-HI-1",
					ProfileLevel: 1,
					Category:     "Host Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL2-CC-CPI-2",
					ProfileLevel: 2,
					Category:     "control plane Isolation",
				},
			},
		},

		{
			testBenchmarks: []*benchmark.Benchmark{
				&benchmark.Benchmark{
					ID:           "MTB-PL2-CC-CPI-2",
					ProfileLevel: 2,
					Category:     "control plane Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL3-CC-HI-2",
					ProfileLevel: 3,
					Category:     "Host Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL2-CC-TI-2",
					ProfileLevel: 2,
					Category:     "Tenant Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL1-CC-TI-1",
					ProfileLevel: 1,
					Category:     "Tenant Isolation",
				},
			},

			expectedBenchmarks: []*benchmark.Benchmark{
				&benchmark.Benchmark{
					ID:           "MTB-PL1-CC-TI-1",
					ProfileLevel: 1,
					Category:     "Tenant Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL2-CC-CPI-2",
					ProfileLevel: 2,
					Category:     "control plane Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL2-CC-TI-2",
					ProfileLevel: 2,
					Category:     "Tenant Isolation",
				},
				&benchmark.Benchmark{
					ID:           "MTB-PL3-CC-HI-2",
					ProfileLevel: 3,
					Category:     "Host Isolation",
				},
			},
		},
	}

	for _, item := range tests {
		sortedBenchmarks := sortBenchmarks(item.testBenchmarks)
		if !reflect.DeepEqual(sortedBenchmarks, item.expectedBenchmarks) {
			t.Errorf("Error in sorting the benchmarks. Output from SortBenchmarks function")
			for _, b := range sortedBenchmarks {
				fmt.Println("ID: ", b.ID)
				fmt.Println("Profile Level: ", string(b.ProfileLevel))
				fmt.Println("Category: ", b.Category)
			}
		}
	}
}
