package reporter

import (
	"fmt"
	"os"
	"strconv"

	v1alpha1 "github.com/kubernetes-sigs/wg-policy-prototypes/policy-report/api/v1alpha1"
	"github.com/olekukonko/tablewriter"
	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/pkg/benchmark"
)

// DefaultReporter collects all the test summaries
type DefaultReporter struct {
	testSummaries []*TestSummary
}

var testResult = map[*benchmark.Benchmark]v1alpha1.PolicyStatus{}

// NewDefaultReporter returns the pointer of DefaultReporter
func NewDefaultReporter() *DefaultReporter {
	return &DefaultReporter{}
}

// SuiteWillBegin prints banner and total benchmarks to be run
func (r *DefaultReporter) SuiteWillBegin(suiteSummary *SuiteSummary) {
	writer.PrintBanner(writer.Colorize(boldStyle, "%s", suiteSummary.Suite.Title), "=")
	writer.Println(0, writer.Colorize(lightGrayColor, "Will run %d of %d", suiteSummary.NumberOfTotalTests, suiteSummary.Suite.Totals()))
}

// TestWillRun prints each test status
func (r *DefaultReporter) TestWillRun(testSummary *TestSummary) {
	if testSummary.Validation {
		writer.Print(0, writer.Colorize(cyanColor, "[PL%d] [%s] ", testSummary.Benchmark.ProfileLevel, testSummary.Benchmark.Category))
		writer.Println(0, testSummary.Benchmark.Title)
		writer.Println(0, writer.Colorize(grayColor, "%s", testSummary.Benchmark.Description))
		if testSummary.Test {
			testResult[testSummary.Benchmark] = "Pass"
			passed := "Passed " + tick
			writer.Println(0, writer.Colorize(greenColor, passed))
		} else {
			testResult[testSummary.Benchmark] = "Fail"
			failed := "Failed " + cross
			writer.Println(0, writer.Colorize(redColor, failed))
			writer.Print(0, writer.Colorize(lilac, "Remediation: "))
			writer.Println(0, writer.Colorize(lightGrayColor, testSummary.Benchmark.Remediation))

		}
		writer.PrintBanner(writer.Colorize(grayColor, "Completed in %v", testSummary.RunTime), "-")
		return
	}
	testResult[testSummary.Benchmark] = "Error"
	preRunfmt := writer.Colorize(magentaColor, "[PreRun-Validation Error]")
	errormsg := writer.Colorize(redColor, testSummary.ValidationError.Error())
	bannerText := fmt.Sprintf("%s %s: %s %s", preRunfmt, testSummary.Benchmark.Title, errormsg, cross)
	writer.PrintBanner(bannerText, "-")
	r.testSummaries = append(r.testSummaries, testSummary)
}

// SuiteDidEnd prints end result summary of benchmark suite
func (r *DefaultReporter) SuiteDidEnd(suiteSummary *SuiteSummary) {
	writer.Print(0, writer.Colorize(greenColor, "%d Passed | ", suiteSummary.NumberOfPassedTests))
	writer.Print(0, writer.Colorize(redColor, "%d Failed | ", suiteSummary.NumberOfFailedTests))
	writer.Print(0, writer.Colorize(yellowColor, "%d Skipped | ", suiteSummary.NumberOfSkippedTests))
	writer.Print(0, writer.Colorize(magentaColor, "%d Errors | ", suiteSummary.NumberOfFailedValidations))
	writer.PrintNewLine()
	writer.PrintBanner(writer.Colorize(grayColor, "Completed in %v", suiteSummary.RunTime), "=")

	printScoreCard(testResult)
}

// FullSummary prints end result of all the tests at one place.
func printScoreCard(testResult map[*benchmark.Benchmark]v1alpha1.PolicyStatus) {
	data := [][]string{}
	counter := 0

	for val, key := range testResult{
		counter++
		var status string
		var symbol string
		
		switch key {
		case "Error":
			status = writer.Colorize(magentaColor, "Error")
			symbol = cross
		case "Pass":
			status = writer.Colorize(greenColor, "Passed")
			symbol = tick
		case "Fail":
			status = writer.Colorize(redColor, "Failed")
			symbol = cross
		case "Skip":
			status = writer.Colorize(yellowColor, "Skipped")
			symbol = skipped
		}

		testName := val.Title + " " + symbol
		result := []string{strconv.Itoa(counter), strconv.Itoa(val.ProfileLevel), testName, status}
		data = append(data, result)
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"No.", "PLevel", "Test", "Result"})
	table.SetAutoWrapText(false)

	for _, v := range data {
		table.Append(v)
	}
	table.Render() // Send output
}
