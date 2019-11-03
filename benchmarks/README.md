# Multi-Tenancy Benchmarks

This repository contains a set of Multi-Tenancy Benchmarks published by the 
[Multi-Tenancy Working Group](https://github.com/kubernetes-sigs/multi-tenancy). The benchmarks can be used to validate if a Kubernetes cluster is properly configured for multi-tenancy. An e2e test tool that can be used to validate if your clusters are multi-tenant, is also provided.

The multi-tenancy benchmarks are meant to be used as guidelines and best practices and as part of a comprehensive security strategy. In other words they are not a substitute for a other security benchmarks, guidelines, or best practices.

For background, see: [Multi-Tenancy Benchmarks Proposal](https://docs.google.com/document/d/1O-G8jEpiJxOeYx9Pd2OuOSb8859dTRNmgBC5gJv0krE/edit?usp=sharing).

## Documentation
- [Multi-Tenancy Definitions](documentation/definitions.md)
- [Benchmark Types](documentation/types.md)
- [Benchmark Categories](documentation/categories.md)
- [Running Validation Tests](documentation/run.md)
- [Contributing](documentation/contributing.md)

## Benchmarks

### Multi-Tenancy Benchmarks Profile Level 1 (MTB-PL1)

*[see profile definitions](documentation/definitions.md#level-1) and [categories](documentation/categories.md).*

#### Configuration Checks (CC)

| ID             | Benchmark                                                                                            | Test  |
|------------------|------------------------------------------------------------------------------------------------------|-------|
| MTB-PL1-CC-CPI-1 | [Namespace resource quotas are configured for each resource type](e2e/tests/resourcequotas/README.md)| --    |


#### Behavioral Checks

| ID | Benchmark                                                                      | Test                            |
|------|--------------------------------------------------------------------------------|---------------------------------|
| MTB-PL1-BC-CPI-2 | [Tenants cannot list cluster-wide resources](e2e/tests/tenantaccess/) | [src](e2e/tests/tenantaccess/tenantaccess.go) |
| MTB-PL1-BC-CPI-3 | [Tenants cannot modify multi-tenancy resources in their namespaces](e2e/tests/modify_admin_resource/README.md)| |
| MTB-PL1-BC-TI-1 | [Tenants cannot list namespaced resources from other tenants](e2e/tests/tenantprotection/README.md) | |
| MTB-PL1-BC-TI-2 | [Tenants cannot modify their resource quotas](e2e/tests/modify_resourcequotas/README.md) | |
| MTB-PL1-BC-NI-1 | [Tenants cannot create network connections to other tenant's pods](e2e/tests/network_isolation/README.md)| |
| MTB-PL1-BC-HI-1 | [Tenants cannot use bind mounts](e2e/tests/deny_hostpaths/README.md) | |
| MTB-PL1-BC-HI-2 | [Tenants cannot use NodePorts](e2e/tests/deny_nodeports/README.md) | |
| MTB-PL1-BC-HI-3 | [Tenants cannot use host networking ](e2e/tests/deny_hostports/README.md) | |

### Multi-Tenancy Profile Level 2

*[see profile definitions](documentation/definitions.md#level-2) and [categories](documentation/categories.md).*


### Multi-Tenancy Profile Level 3

*[see profile definitions](documentation/definitions.md#level-3) and [categories](documentation/categories.md).*

