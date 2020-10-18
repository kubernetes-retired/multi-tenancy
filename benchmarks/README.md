# Multi-Tenancy Benchmarks

Multi-Tenancy Benchmarks (MTB) are guidelines for multi-tenant configuration of Kubernetes clusters. 

The kubectl plugin [`kubectl-mtb`](kubectl-mtb/README.md) can be used to validate if a Kubernetes cluster is properly configured for multi-tenancy.

Multi-Tenancy Benchmarks are meant to be used as part of a comprehensive security strategy. They are not a substitute for other security best practices and do not guarantee security.

For background, see: [Multi-Tenancy Benchmarks Proposal](https://docs.google.com/document/d/1O-G8jEpiJxOeYx9Pd2OuOSb8859dTRNmgBC5gJv0krE/edit?usp=sharing).


## Status

***The multi-tenancy benchmarks are in development and not ready for usage.***

## Documentation
- [Multi-tenancy Definitions](documentation/definitions.md)
- [Benchmark Profiles](documentation/definitions.md#multi-tenancy-profiles)
- [Benchmark Types](documentation/types.md)
- [Benchmark Categories](documentation/categories.md)
- [Running benchmark validation tests with kubectl-mtb](kubectl-mtb/README.md)
- [Contributing to the benchmarks](kubectl-mtb/README.md#contributing)

## Benchmarks

The following tests are currently defined (tests marked `pending` are planned for implementation):

### Profile Level 1

* [Block access to cluster resources](kubectl-mtb/test/benchmarks/block_access_to_cluster_resources)
* [Block access to Multitenant Resources](kubectl-mtb/test/benchmarks/block_multitenant_resources)
* Block access to other tenant resources (**pending** [#1197](https://github.com/kubernetes-sigs/multi-tenancy/issues/1197))
* [Block add capabilities](kubectl-mtb/test/benchmarks/block_add_capabilities)
* [Require image pull `always`](kubectl-mtb/test/benchmarks/require_always_pull_image)
* [Require run as non-root user](kubectl-mtb/test/benchmarks/require_run_as_non_root_user)
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
* Require PersistentVolumeClaim for storage (**pending** [#1198](https://github.com/kubernetes-sigs/multi-tenancy/issues/1198))
* Require PV reclaim policy of `delete` (**pending** [#1199](https://github.com/kubernetes-sigs/multi-tenancy/issues/1199))
* Block use of existing PVs (**pending** [#1200](https://github.com/kubernetes-sigs/multi-tenancy/issues/1200))
* Block network access across tenant namespaces (**pending** [#1201](https://github.com/kubernetes-sigs/multi-tenancy/issues/1201))

### Profile Level 2

* [Allow self-service management of Network Policies](kubectl-mtb/test/benchmarks/create_network_policies)
* Allow self-service management of Roles (**pending** [#1202](https://github.com/kubernetes-sigs/multi-tenancy/issues/1202))
* Allow self-service management of Roles Bindings (**pending** [#1203](https://github.com/kubernetes-sigs/multi-tenancy/issues/1203))

