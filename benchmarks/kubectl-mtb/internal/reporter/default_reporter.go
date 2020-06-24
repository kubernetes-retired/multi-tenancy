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
		writer.Println(0, writer.Colorize(cyanColor, "[PL%d] [%s] %s", testSummary.Benchmark.ProfileLevel, testSummary.Benchmark.Category, testSummary.Benchmark.Title))
		writer.Println(0, writer.Colorize(grayColor, "%s", testSummary.Benchmark.Description))
		if testSummary.Test {
			writer.Println(0, writer.Colorize(greenColor, "Passed"))
		} else {
			writer.Println(0, writer.Colorize(redColor, "Failed"))
			writer.Println(0, writer.Colorize(lightGrayColor, "Remediation: %s", testSummary.Benchmark.Remediation))

		}
		writer.PrintBanner(writer.Colorize(grayColor, "Completed in %v", testSummary.RunTime), "-")
		return
	}

	writer.Println(0, writer.Colorize(yellowColor, "Skipping %s: %v", testSummary.Benchmark.Title, testSummary.ValidationError))
	r.testSummaries = append(r.testSummaries, testSummary)
}

func (r *DefaultReporter) SuiteDidEnd(suiteSummary *SuiteSummary) {
	writer.Print(0, writer.Colorize(greenColor, "%d Passed | ", suiteSummary.NumberOfPassedTests))
	writer.Print(0, writer.Colorize(redColor, "%d Failed | ", suiteSummary.NumberOfFailedTests))
	writer.Print(0, writer.Colorize(yellowColor, "%d Skipped | ", suiteSummary.NumberOfSkippedTests))
	writer.PrintNewLine()
	writer.PrintBanner(writer.Colorize(grayColor, "Completed in %v", suiteSummary.RunTime), "=")
}
