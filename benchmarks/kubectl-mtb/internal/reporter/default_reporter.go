package reporter

import (
	"os"

	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/internal/reporter/printer"
)

const defaultStyle = "\x1b[0m"
const boldStyle = "\x1b[1m"
const redColor = "\x1b[91m"
const greenColor = "\x1b[32m"
const yellowColor = "\x1b[33m"
const cyanColor = "\x1b[36m"
const grayColor = "\x1b[90m"
const lightGrayColor = "\x1b[37m"

type DefaultReporter struct {
	testSummaries []*TestSummary
}

var writer = printer.NewConsoleLogger(true, os.Stdout)

func NewDefaultReporter() *DefaultReporter {
	return &DefaultReporter{}
}

func (r *DefaultReporter) SuiteWillBegin(suiteSummary *SuiteSummary) {
	writer.PrintBanner(writer.Colorize(boldStyle, "%s", suiteSummary.Suite.Title), "=")
	writer.Println(0, writer.Colorize(lightGrayColor, "Will run %d of %d", suiteSummary.NumberOfTotalTests, suiteSummary.Suite.Totals()))
}

func (r *DefaultReporter) TestWillRun(testSummary *TestSummary) {
	if testSummary.Validation {
		writer.Println(0, writer.Colorize(cyanColor, "%s", testSummary.Benchmark.Title))
		writer.Println(0, writer.Colorize(grayColor, "%s", testSummary.Benchmark.Description))
		writer.PrintBanner(writer.Colorize(grayColor, "Completed in %v", testSummary.RunTime), "-")
		return
	}

	writer.Println(0, writer.Colorize(yellowColor, "Skipping %s: %v", testSummary.Benchmark.Title, testSummary.ValidationError))
}

func (r *DefaultReporter) SuiteDidEnd(suiteSummary *SuiteSummary) {

}
