package kubectl

import (
	"fmt"
	"reflect"

	"github.com/spf13/cobra"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
)

func getResource(args []string) (string, error) {
	if len(args) == 0 {
		return "benchmarks", nil
	}

	r := args[0]
	if !supportedResourceNames.Has(r) {
		return "", fmt.Errorf("Please specify one of the following resources: %v", reflect.ValueOf(supportedResourceNames).MapKeys())
	}

	return r, nil
}

func getBenchmarkArg(args []string) string {
	if len(args) != 2 {
		return ""
	}

	return args[1]
}

func filterBenchmarks(cmd *cobra.Command, args []string) error {
	profileLevel, _ := cmd.Flags().GetInt("profile-level")

	id := getBenchmarkArg(args)
	if id != "" {
		b := test.BenchmarkSuite.ID(id)
		if b == nil {
			return fmt.Errorf("unknown benchmark ID: %s", id)
		}
		benchmarks = []*benchmark.Benchmark{b}
	} else {
		benchmarks = test.BenchmarkSuite.ProfileLevel(profileLevel)
	}
	return nil
}
