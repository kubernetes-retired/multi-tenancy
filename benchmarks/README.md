# Multi-Tenancy Benchmarks

Multi-Tenancy Benchmarks (MTB) are guidelines for multi-tenant configuration of Kubernetes clusters. 

The kubectl plugin [`kubectl-mtb`](kubectl-mtb/README.md) can be used to validate if a Kubernetes cluster is properly configured for multi-tenancy.

Multi-Tenancy Benchmarks are meant to be used as part of a comprehensive security strategy. They are not a substitute for other security best practices and do not guarantee security.

For background, see: [Multi-Tenancy Benchmarks Proposal](https://docs.google.com/document/d/1O-G8jEpiJxOeYx9Pd2OuOSb8859dTRNmgBC5gJv0krE/edit?usp=sharing).


## Status

***The multi-tenancy benchmarks are in development and not ready for usage.***

## Documentation
- [Multi-Tenancy Definitions](documentation/definitions.md)
- [Benchmark Types](documentation/types.md)
- [Benchmark Categories](documentation/categories.md)
- [Running benchmark conformance tests with kubectl-mtb](kubectl-mtb/README.md)
- [Contributing to the benchmarks](kubectl-mtb/README.md#contributing)

## Benchmarks

The following tests are currently defined (tests marked `pending` are planned for implementation):

### Profile Level 1

* [Block access to cluster resources](kubectl-mtb/test/benchmarks/block_access_to_cluster_resources)
* [Block Multitenant Resources](kubectl-mtb/test/benchmarks/block_multitenant_resources)
* [Block add capabilities](kubectl-mtb/test/benchmarks/block_add_capabilities)
* Require run as non-root user (**pending**)
* Require image pull `always` (**pending**)
* Require PVC reclaim policy `delete` (**pending**)
* Require CAP_DROP_ALL (**pending**)
* [Block privileged containers](kubectl-mtb/test/benchmarks/block_privileged_containers)
* [Block privilege escalation](kubectl-mtb/test/benchmarks/block_privilege_escalation)
* [Configure namespace resource quotas](kubectl-mtb/test/benchmarks/configure_ns_quotas)
* [Configure namespace object limits](kubectl-mtb/test/benchmarks/configure_ns_object_quota)
* [Block use of host path volumes](kubectl-mtb/test/benchmarks/block_use_of_host_path)
* [Block use of NodePort services](kubectl-mtb/test/benchmarks/block_use_of_nodeport_services)
* [Block use of host networking and ports](kubectl-mtb/test/benchmarks/block_use_of_host_networking_and_ports)
* [Block use of host PID](kubectl-mtb/test/benchmarks/block_use_of_host_pid)
* [Block use of host IPC](kubectl-mtb/test/benchmarks/block_use_of_host_ipc)
* [Block modification of resource quotas](kubectl-mtb/test/benchmarks/block_ns_quota)

### Profile Level 2

* [Create Role Bindings](kubectl-mtb/test/benchmarks/create_role_bindings)
* [Create Network Policies](kubectl-mtb/test/benchmarks/create_network_policies)
* Create Namespaces (**pending**)

