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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/internal/reporter"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test/utils/log"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/types"
)

var benchmarkRunOptions = types.RunOptions{}

var runCmd = &cobra.Command{
	Use:   "run <resource>",
	Short: "run one or more multi-tenancy benchmarks",

	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if _, err := getResource(args); err != nil {
			return err
		}

		err := validateFlags(cmd)
		if err != nil {
			return err
		}

		filterBenchmarks(cmd, args)
		return nil

	},

	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(runTests(cmd, args))
	},
}

func initConfig() error {
	kubecfgFlags := genericclioptions.NewConfigFlags(false)
	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		return err
	}

	// create the K8s clientset
	benchmarkRunOptions.ClusterAdminClient, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	tenantConfig := config
	tenantConfig.Impersonate.UserName = benchmarkRunOptions.Tenant

	// create the tenant clientset
	benchmarkRunOptions.Tenant1Client, err = kubernetes.NewForConfig(tenantConfig)
	if err != nil {
		return err
	}

	if benchmarkRunOptions.OtherNamespace != "" && benchmarkRunOptions.OtherTenant != "" {
		otherTenantConfig := config
		otherTenantConfig.Impersonate.UserName = benchmarkRunOptions.OtherTenant

		benchmarkRunOptions.Tenant2Client, err = kubernetes.NewForConfig(tenantConfig)
		if err != nil {
			return err
		}
	}

	return err
}

func reportSuiteWillBegin(suiteSummary *reporter.SuiteSummary, reportersArray []reporter.Reporter) {
	for _, reporter := range reportersArray {
		reporter.SuiteWillBegin(suiteSummary)
	}
}

func reportTestWillRun(testSummary *reporter.TestSummary, reportersArray []reporter.Reporter) {
	for _, reporter := range reportersArray {
		reporter.TestWillRun(testSummary)
	}
}

func reportSuiteDidEnd(suiteSummary *reporter.SuiteSummary, reportersArray []reporter.Reporter) {
	for _, reporter := range reportersArray {
		reporter.SuiteDidEnd(suiteSummary)
	}
}

func removeBenchmarksWithIDs(ids []string) {
	var temp []*benchmark.Benchmark
	for _, benchmark := range benchmarks {
		found := false
		for _, id := range ids {
			if benchmark.ID == id {
				found = true
			}
		}

		if !found {
			temp = append(temp, benchmark)
		}
	}
	benchmarks = temp
}

// Validation of the flag inputs
func validateFlags(cmd *cobra.Command) error {
	tenants, _ := cmd.Flags().GetStringSlice("as")
	tenantNamespaces, _ := cmd.Flags().GetStringSlice("namespace")

	if len(tenants) < 1 || len(tenantNamespaces) < 1 {
		return fmt.Errorf("user and namespace required")
	}

	if len(tenants) != len(tenantNamespaces) {
		return fmt.Errorf("user and namespace counts must be equal")
	}

	if len(tenants) > 2 || len(tenantNamespaces) > 2 {
		return fmt.Errorf("user and namespace counts cannot exceed 2")
	}

	benchmarkRunOptions.Tenant = tenants[0]
	benchmarkRunOptions.TenantNamespace = tenantNamespaces[0]

	if len(tenants) > 1 {
		benchmarkRunOptions.OtherTenant = tenants[1]
		benchmarkRunOptions.OtherNamespace = tenantNamespaces[1]
	}

	err := initConfig()
	if err != nil {
		return err
	}

	_, err = benchmarkRunOptions.ClusterAdminClient.CoreV1().Namespaces().Get(context.TODO(), benchmarkRunOptions.TenantNamespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("invalid namespace")
	}

	return nil
}

func setupLogger(cmd *cobra.Command) {
	debug, _ := cmd.Flags().GetBool("debug")
	if debug {
		benchmarkRunOptions.Logger = log.GetLogger(true)
	} else {
		// default mode production
		benchmarkRunOptions.Logger = log.GetLogger(false)
	}
}

func setupReporters(cmd *cobra.Command) ([]reporter.Reporter, error) {
	// Get reporters from the user
	reporterFlag, _ := cmd.Flags().GetString("out")
	reporters := strings.Split(reporterFlag, ",")
	return reporter.GetReporters(reporters)
}

func executePreRun(b *benchmark.Benchmark, suiteSummary *reporter.SuiteSummary, ts *reporter.TestSummary) {
	err := b.PreRun(benchmarkRunOptions)
	if err != nil {
		benchmarkRunOptions.Logger.Debug(err.Error())
		suiteSummary.NumberOfFailedValidations++
		ts.Validation = false
		ts.ValidationError = err
		b.Status = "Error"
	}
}

func executeRun(b *benchmark.Benchmark, suiteSummary *reporter.SuiteSummary, ts *reporter.TestSummary) {
	if ts.Validation {
		err := b.Run(benchmarkRunOptions)
		if err != nil {
			benchmarkRunOptions.Logger.Debug(err.Error())
			suiteSummary.NumberOfFailedTests++
			ts.Test = false
			ts.TestError = err
			b.Status = "Fail"
		} else {
			suiteSummary.NumberOfPassedTests++
			b.Status = "Pass"
		}
	}
}

func executePostRun(b *benchmark.Benchmark, suiteSummary *reporter.SuiteSummary, ts *reporter.TestSummary) {
	if ts.Test {
		if b.PostRun != nil {
			err := b.PostRun(benchmarkRunOptions)
			if err != nil {
				fmt.Print(err.Error())
			}
		}
	}
}

func shouldSkipTest(b *benchmark.Benchmark, suiteSummary *reporter.SuiteSummary, ts *reporter.TestSummary) bool {
	if b.NamespaceRequired > 1 {
		if benchmarkRunOptions.OtherNamespace != "" && benchmarkRunOptions.OtherTenant != "" {
			return false
		}
		return true
	}
	return false
}

func runTests(cmd *cobra.Command, args []string) error {

	benchmarkRunOptions.Label, _ = cmd.Flags().GetString("labels")
	// Get log level
	setupLogger(cmd)

	reportersArray, err := setupReporters(cmd)
	if err != nil {
		return err
	}

	// Get benchmark ids from the user to skip them
	skipFlag, _ := cmd.Flags().GetString("skip")
	skipIDs := strings.Split(skipFlag, ",")
	removeBenchmarksWithIDs(skipIDs)

	suiteSummary := &reporter.SuiteSummary{
		Suite:              test.BenchmarkSuite,
		NumberOfTotalTests: len(benchmarks),
		Namespace:          benchmarkRunOptions.TenantNamespace,
		User:               benchmarkRunOptions.Tenant,
	}

	suiteStartTime := time.Now()
	reportSuiteWillBegin(suiteSummary, reportersArray)

	for _, b := range benchmarks {

		ts := &reporter.TestSummary{
			Benchmark: b,
		}

		err := ts.SetDefaults()
		if err != nil {
			benchmarkRunOptions.Logger.Debug(err.Error())
			return err
		}

		startTest := time.Now()

		if shouldSkipTest(b, suiteSummary, ts) {
			continue
		}

		// Lifecycles
		executePreRun(b, suiteSummary, ts)

		executeRun(b, suiteSummary, ts)

		executePostRun(b, suiteSummary, ts)

		elapsed := time.Since(startTest)
		ts.RunTime = elapsed
		reportTestWillRun(ts, reportersArray)
	}

	suiteElapsedTime := time.Since(suiteStartTime)
	suiteSummary.RunTime = suiteElapsedTime
	suiteSummary.NumberOfSkippedTests = test.BenchmarkSuite.Totals() - len(benchmarks)
	reportSuiteDidEnd(suiteSummary, reportersArray)

	return nil
}

func newRunCmd() *cobra.Command {
	runCmd.Flags().BoolP("debug", "d", false, "Use debugging mode")
	runCmd.Flags().StringSliceP("namespace", "n", []string{}, "(required) tenant namespace")
	runCmd.Flags().StringSlice("as", []string{}, "(required) user name to impersonate")
	runCmd.Flags().StringP("out", "o", "default", "(optional) output reporters (default, policyreport)")
	runCmd.Flags().StringP("skip", "s", "", "(optional) benchmark IDs to skip")
	runCmd.Flags().StringP("labels", "l", "", "(optional) labels")

	return runCmd
}
