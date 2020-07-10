package reporter

const defaultStyle = "\x1b[0m"
const boldStyle = "\x1b[1m"
const redColor = "\x1b[91m"
const greenColor = "\x1b[32m"
const yellowColor = "\x1b[33m"
const cyanColor = "\x1b[36m"
const grayColor = "\x1b[90m"
const magentaColor = "\033[35m"
const lightGrayColor = "\x1b[37m"

// DefaultReporter collects all the test summaries
type DefaultReporter struct {
	testSummaries []*TestSummary
}

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

	writer.Println(0, writer.Colorize(magentaColor, "PreRun-Validation Error %s: %v", testSummary.Benchmark.Title, testSummary.ValidationError))
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
}
