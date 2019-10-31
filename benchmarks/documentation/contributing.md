# Contributing Guidelines

The overall contributing guidelines can be found [here](../../CONTRIBUTING.md).

## Getting Started

- To add a new benchmark test, submit an issue using the [template](../.github/ISSUE_TEMPLATE/new-benchmark-test.md) to describe the test case
- To start implementing the test in [benchmark suite](../README.md##benchmarks):
    - navigate to the the particular work dir or create a new one in [e2e/tests/](../e2e/tests)
    - add code to the work dir
    - add the test manifest in [e2e.go](../e2e/tests/e2e.go#L12) to pre-compile the resource