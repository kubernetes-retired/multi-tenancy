package reporter

type Reporter interface {
	SuiteWillBegin(*SuiteSummary)
	TestWillRun(*TestSummary)
	SuiteDidEnd(*SuiteSummary)
}
