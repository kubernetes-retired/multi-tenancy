package reporter

import (
	"os"

	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/internal/reporter/printer"
)

// Reporter defines the lifecycle of reporter
type Reporter interface {
	SuiteWillBegin(suiteSummary *SuiteSummary)
	TestWillRun(testSummary *TestSummary)
	SuiteDidEnd(suiteSummary *SuiteSummary)
}

// GetReporters returns the Reporter array as per the user input
func GetReporters(reporters []string) ([]Reporter, error) {
	var reportersArray []Reporter

	// Add the default reporter
	reportersArray = append(reportersArray, NewDefaultReporter())

	for _, r := range reporters {
		switch r {
		case "policyreport":
			reportersArray = append(reportersArray, NewPolicyReporter())
		}
	}
	return reportersArray, nil
}

// Hard coded the color bool value
var writer = printer.NewConsoleLogger(true, os.Stdout)

const defaultStyle = "\x1b[0m"
const boldStyle = "\x1b[1m"
const redColor = "\x1b[91m"
const greenColor = "\x1b[32m"
const yellowColor = "\x1b[33m"
const cyanColor = "\x1b[36m"
const grayColor = "\x1b[90m"
const magentaColor = "\033[35m"
const lightGrayColor = "\x1b[37m"
const lilac = "\033[38;2;200;162;200m"
const tick = "\u2705"
const cross = "\u274c"
const skipped = "\u23ed"