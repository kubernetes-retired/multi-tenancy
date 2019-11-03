# Contributing Guidelines

The overall Kubernetes contributing guidelines can be found [here](../../CONTRIBUTING.md).

## Getting Started

Thanks for your interest in contributing to the multi-tenancy benchmarks. There are several ways you can help! 

1. To request a new benchmark and validation test, submit an issue using the [template](../.github/ISSUE_TEMPLATE/new-benchmark-test.md)

2. Improve an existing benchmark
     - Update the README.md for the benchmark and submit a PR.

3. Add or improve the validation test for an existing:
     - Check the [benchmark suite](../README.md##benchmarks) for benchmarks that are missing validation tests. 
     - Find the folder for the benchmark under  [e2e/tests/](../e2e/tests)
     - Add or modify the source code and submit a PR.

4. Submit a new benchmark and validation test:
    - Pick the right benchmark type and category and make sure a similar test does not exist which can be improved to cover your needs.
    - Create a new directory in [e2e/tests/](../e2e/tests)
    - Add a README.md with using the template provided [below](#benchmark-template).
    - add the test manifest in [e2e.go](../e2e/tests/e2e.go#L12) to pre-compile the resource
    - once your benchmark is accepted, the maintainers will assign a test suite ID and update the main index.
    
   
## Benchmark Template

````yaml

# <A short and descriptive benchmark title>

**Profile Applicability:**

Level 1, or Level 2, Level 3 (see documentation for definitions)

**Type:**

Behavioral, or Configuration (see documentation for definitions)

**Category:**

One of the define categories such as Control Plane Isolation, Tenant Isolation, Network Isolation, etc. See documentation for the complete list.

**Description:**

A description what the benchmark advocates or valdidates.

**Rationale:**

A description why the benchmark is required for multi-tenancy.

**Audit:**

Demonstrate how the benchmark can be checked via kubectl. For example:

Run the following commands to retrieve the list of non namespaced resources:

  	kubectl --kubeconfig cluster-admin api-resources --namespaced=false

For all non namespaced resources,  issue the following command:

        kubectl --kubeconfig tenant-a get <resource>

Each command must return 403 FORBIDDEN

````


<br/><br/>
*Back to >> [Documentation](../README.md)*
