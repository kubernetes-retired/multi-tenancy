# Multi-Tenancy Benchmarks

This repository contains a set of Multi-Tenancy Benchmarks published by the 
[Multi-Tenancy Working Group](https://github.com/kubernetes-sigs/multi-tenancy). The benchmarks can be used to validate if a Kubernetes cluster is properly configured for multi-tenancy. An e2e test tool that can be used to validate if your clusters are multi-tenant, is also provided.

The multi-tenancy benchmarks are meant to be used as guidelines and best practices and as part of a comprehensive security strategy. In other words they are not a substitute for a other security benchmarks, guidelines, or best practices.

For background, see: [Multi-Tenancy Benchmarks Proposal](https://docs.google.com/document/d/1O-G8jEpiJxOeYx9Pd2OuOSb8859dTRNmgBC5gJv0krE/edit?usp=sharing).


## Status

***The multi-tenancy benchmarks are in development and not ready for usage.***

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
| MTB-PL1-CC-CPI-1 | [Block access to cluster resources](e2e/tests/block_cluster_resources/) | [src](e2e/tests/block_cluster_resources/block_cluster_resources.go) |
| MTB-PL1-CC-TI-2 | [Block access to other tenant resources](e2e/tests/block_other_tenant_resources/) | |
| MTB-PL1-CC-FNS-1 | [Configure namespace resource quotas](e2e/tests/configure_ns_quotas/)|  [src](e2e/tests/configure_ns_quotas/configure_ns_quotas.go) |
| MTB-PL1-CC-TI-1 | [Block modification of resource quotas](e2e/tests/block_ns_quotas/) | |

#### Behavioral Checks (BC)

| ID | Benchmark                                                                      | Test                            |
|------|--------------------------------------------------------------------------------|---------------------------------|
| MTB-PL1-BC-CPI-2 | [Block modification of multi-tenancy resources](e2e/tests/block_multitenant_resources/)| |
| MTB-PL1-BC-CPI-3 | [Block add capabilities](e2e/tests/block_add_capabilities/)  | |
| MTB-PL1-BC-CPI-4 | [Require running as non-root user](e2e/tests/require_run_as_non_root/)  | |
| MTB-PL1-BC-CPI-5 | [Block privileged containers](e2e/tests/block_privileged_containers/)  | |
| MTB-PL1-BC-CPI-6 | Block privilege escalation  | |
| MTB-PL1-BC-NI-1 | [Default deny network connections across tenants](e2e/tests/default_deny_net_conn/)| |
| MTB-PL1-BC-HI-1 | [Block use of bind mounts](e2e/tests/block_bind_mounts/) | |
| MTB-PL1-BC-HI-2 | [Block use of NodePort services](e2e/tests/block_nodeports/) | |
| MTB-PL1-BC-HI-3 | [Block use of host networking and ports ](e2e/tests/block_host_net_ports/) | |
| MTB-PL1-BC-HI-4 | [Block use of host PID](e2e/tests/block_host_pid/)  | |
| MTB-PL1-BC-HI-5 | [Block use of host IPC](e2e/tests/block_host_ipc/))  | |

### Multi-Tenancy Profile Level 2

*[see profile definitions](documentation/definitions.md#level-2) and [categories](documentation/categories.md).*


### Multi-Tenancy Profile Level 3

*[see profile definitions](documentation/definitions.md#level-3) and [categories](documentation/categories.md).*

