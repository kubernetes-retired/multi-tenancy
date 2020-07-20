package reporter

import (
	"fmt"
	"os"

	"sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb/internal/reporter/printer"
)

// Reporter defines the lifecycle of reporter
type Reporter interface {
	SuiteWillBegin(suiteSummary *SuiteSummary)
	TestWillRun(testSummary *TestSummary)
	SuiteDidEnd(suiteSummary *SuiteSummary)
	FullSummary(finalSummary *FinalSummary)
}

// GetReporter returns the Reporter pointer as per the user input
func GetReporter(reporter string) (Reporter, error) {
	switch reporter {
	case "default":
		return NewDefaultReporter(), nil

	case "policy":
		return NewPolicyReporter(), nil
	}

	return nil, fmt.Errorf("Wrong reporter value passed")
}

// Hard coded the color bool value
var writer = printer.NewConsoleLogger(true, os.Stdout)
