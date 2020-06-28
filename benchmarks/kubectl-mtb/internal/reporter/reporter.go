package reporter

import "fmt"

// Reporter defines the lifecycle of reporter
type Reporter interface {
	SuiteWillBegin(suiteSummary *SuiteSummary)
	TestWillRun(testSummary *TestSummary)
	SuiteDidEnd(suiteSummary *SuiteSummary)
}

// GetReporter returns the Reporter pointer as per the user input
func GetReporter(reporter string) (Reporter, error) {
	switch reporter {
	case "default":
		return NewDefaultReporter(), nil
	}

	return nil, fmt.Errorf("Wrong reporter value passed")
}
