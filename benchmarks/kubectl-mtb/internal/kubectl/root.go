/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package kubectl

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
)

var rootCmd *cobra.Command
var maxProfileLevel = 3
var benchmarks []*benchmark.Benchmark

// singular-plural
var supportedResourceNames = sets.NewString("benchmarks", "benchmark")

func init() {

	// rootCmd represents the base command when called without any subcommands
	rootCmd = &cobra.Command{
		Use:   "kubectl-mtb",
		Short: "Multi-Tenancy Benchmarks",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

			validateResource(args)

			// Initiate new suite instance
			bs := test.NewBenchmarkSuite()

			profileLevel, _ := cmd.Flags().GetInt("profile-level")
			benchmarks = bs.ProfileLevel(profileLevel)

			return nil
		},
	}

	rootCmd.PersistentFlags().StringP("category", "c", "", "Category of the benchmarks.")
	rootCmd.PersistentFlags().IntP("profile-level", "p", maxProfileLevel, "ProfileLevel of the benchmarks.")

	// Commands
	rootCmd.AddCommand(newGetCmd())
	rootCmd.AddCommand(newTestCmd())
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func validateResource(args []string) {
	if len(args) == 0 {
		fmt.Println("Error: Please specify any resource")
		os.Exit(1)
	}
	if !supportedResourceNames.Has(args[0]) {
		fmt.Println("Error: Please specify any valid resource.")
		os.Exit(1)
	}
}
