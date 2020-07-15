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
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/internal/reporter"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/test"
)

var (
	tenant          string
	tenantNamespace string
)

var testCmd = &cobra.Command{
	Use:   "test",
	Short: "Run the Multi-Tenancy Benchmarks",

	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(validateFlags(cmd))
		cmdutil.CheckErr(runTests(cmd, args))
	},
}

// Validation of the flag inputs
func validateFlags(cmd *cobra.Command) error {
	tenant, _ = cmd.Flags().GetString("tenant-admin")
	if tenant == "" {
		return fmt.Errorf("tenant-admin must be set via --tenant-admin or -t")
	}

	tenantNamespace, _ = cmd.Flags().GetString("namespace")
	if tenantNamespace == "" {
		return fmt.Errorf("tenant namespace must be set via --namespace or -n")
	}
	return nil
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

func runTests(cmd *cobra.Command, args []string) error {

	kubecfgFlags := genericclioptions.NewConfigFlags(false)

	config, err := kubecfgFlags.ToRESTConfig()
	if err != nil {
		return err
	}

	// create the K8s clientset
	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	tenantConfig := config
	tenantConfig.Impersonate.UserName = tenant

	// create the tenant clientset
	tenantClient, err := kubernetes.NewForConfig(tenantConfig)
	if err != nil {
		return err
	}

	// Get reporter from the user
	reporters, _ := cmd.Flags().GetString("out")
	reportersArray, err := reporter.GetReporters(strings.Split(reporters, ","))
	if err != nil {
		return err
	}

	suiteSummary := &reporter.SuiteSummary{
		Suite:                test.BenchmarkSuite,
		NumberOfTotalTests:   len(benchmarks),
		TenantAdminNamespace: tenantNamespace,
	}

	suiteStartTime := time.Now()
	reportSuiteWillBegin(suiteSummary, reportersArray)

	for _, b := range benchmarks {

		ts := &reporter.TestSummary{
			Benchmark: b,
		}

		err := ts.SetDefaults()
		if err != nil {
			return err
		}

		startTest := time.Now()

		// Run Prerun
		err = b.PreRun(tenantNamespace, k8sClient, tenantClient)
		if err != nil {
			suiteSummary.NumberOfFailedValidations++
			ts.Validation = false
			ts.ValidationError = err
		}

		// Check PreRun status
		if ts.Validation {
			err = b.Run(tenantNamespace, k8sClient, tenantClient)
			if err != nil {
				suiteSummary.NumberOfFailedTests++
				ts.Test = false
				ts.TestError = err
			} else {
				suiteSummary.NumberOfPassedTests++
			}
		}

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

func newTestCmd() *cobra.Command {
	testCmd.Flags().StringP("namespace", "n", "", "name of tenant-admin namespace")
	testCmd.Flags().StringP("tenant-admin", "t", "", "name of tenant service account")
	testCmd.Flags().StringP("out", "o", "default", "output reporter format")

	return testCmd
}
