# Multi-tenancy-benchmarks

This repository contains a set of Multi-Tenancy Benchmarks published by the 
[Multi-Tenancy Working Group](https://github.com/kubernetes-sigs/multi-tenancy). These benchmarks can be used to validate 
if a Kubernetes cluster is properly configured for multi-tenancy. A e2e validation tool is also provided.

For background, see: [Multi-Tenancy Benchmarks Proposal](https://docs.google.com/document/d/1O-G8jEpiJxOeYx9Pd2OuOSb8859dTRNmgBC5gJv0krE/edit?usp=sharing).

## Documentation
- [Multi-Tenancy Definitions](documentation/definitions.md)
- [Benchmark Types](documentation/types.md)
- [Benchmark Categories](documentation/catagories.md)
- [Running the Validation Tests](documentation/run.md)
- [Roadmap](documentation/roadmap.md)
- [Contributing](documentation/contributing.md)

## Benchmarks

### Multi-Tenancy Profile Level 1

*[see definition](documentation/definitions.md#level-1)*

#### Configuration Checks

| Test       | Benchmark                                                                                            | Code  |
|------------|------------------------------------------------------------------------------------------------------|-------|
| MTB-PL1-C1 | [Namespace resource quotas are configured for each resource type](e2e/tests/resourcequotas/README.md)| --    |


#### Behavioral Checks

| Test | Benchmark                                                                      | Code                            |
|------|--------------------------------------------------------------------------------|---------------------------------|
| MTB-PL1-B1 | [Tenants cannot list cluster-wide resources](e2e/tests/tenantaccess/README.go) | [done](e2e/tests/tenantaccess/tenantaccess.go) |
| MTB-PL1-B2 | [Tenants cannot list namespaced resources from other tenants](e2e/tests/tenantprotection) | |
| MTB-PL1-B3 | [Tenants cannot modify their resource quotas](e2e/tests/modify_resourcequotas) | |
| MTB-PL1-B4 | [Tenants cannot modify multi-tenancy resources in their namespaces](e2e/tests/modify_admin_resource/README.md)| |
| MTB-PL1-B5 | [Tenants cannot create network connections to other tenant's pods](e2e/tests/network_isolation)| |
| MTB-PL1-B6 | [Tenants cannot use bind mounts](e2e/tests/deny_hostpaths) | |
| MTB-PL1-B7 | [Tenants cannot use NodePorts](e2e/tests/deny_nodeports) | |
| MTB-PL1-B8 | [Tenants annot use host networking ](e2e/tests/deny_hostports/README.md) | |

### Multi-Tenancy Profile Level 2

*[see definition](documentation/definitions.md#level-2)*


### Multi-Tenancy Profile Level 3

*[see definition](documentation/definitions.md#level-3)*

