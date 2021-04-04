# Sharing a Super Cluster Pool

In virtualcluster, a key assumption is that the Pod resource provider is a single super cluster which
greatly simplifies the syncer design. This also means that the super cluster needs to equip with reliable
autoscaling capability in order to successfully handle burst tenant resource requests if any. In cases
where worker nodes cannot be automatically added or removed from the super clusters, supporting
multiple super clusters, i.e., a super cluster pool, can be a reasonable alternative to extend the
aggregated resource capacity. As an experimental feature, we enhance the original virtualcluster
architecture to support sharing a pool of super clusters in the same multi-tenancy context.

## Overview

The super cluster pool support is achieved through the following new enhancements and components:

- **Syncer Enhancement**: Under the protection of a new feature gate `SuperClusterPooling`, the syncer can
selectively synchronize the objects from tenant control planes based on their cluster ownerships
(labeled by the schedulers mentioned below). The prerequisite is that each super cluster needs to
install a dedicated syncer which watchs all tenant control planes as usual.
  
- **Namespace Scheduler**: A scheduler that determines the super clusters backing the tenant objects 
for each tenant namespace with CPU/memory quota specified.
A namespace quota is divided into multiple slices (analogous to memory pages in OS scheduling) and the scheduler
distributes the quota slices across the super clusters. This would largely mitigate the potential resource 
fragmentation had a full quota been considered in scheduling. Once a tenant namespace is scheduled to multiple
super clusters, the syncers in those clusters will synchronize all objects in the tenant namespace
except the Pod objects.

- **Pod Scheduler**: Based on the Pod's namespace scheduling result, this scheduler picks one super cluster to
run the Pod. Only the syncer in the scheduled super cluster will synchronize the Pod object.

## Quick Start

Please follow this [demo](./doc/demo.md) as a quick start.

## FAQ

### Q: Is the tenant namespace quota required in the solution?

Yes. Since we assume that super clusters cannot be automatically scaled based on available capacities,
meaning the aggregated cluster capacity is limited, and the super clusters are 
abstracted away from a tenant's perspectives, tenants have to provide hints to their
resource requirements so that a proper capacity distribution can be done by the namespace scheduler.
The native namespace quota is chosen to ease the user experience. A formal CRD could be introduced to
provide a better abstraction. Setting namespace quota is the ONLY required step for a tenant 
to use the super cluster pool.

### Q: Is Service supported?

The ClusterIP type of service cannot work if the endpoints are spread across multiple clusters.
We expect the LoadBalancer type of service will still work since the tenant control plane has the
full endpoints information.

### Q: What if super cluster hosts fail?

The syncer ensures the correct behaviors for the tenant control plane to deal with host failures.
The namespace scheduler does the following to mitigate the impact of super cluster host failures:
- Optionally leave some headrooms when reporting super cluster capacity to tolerate some host failures.
- Unlike Pod scheduling, the namespace scheduling result can be overwritten or cleared for rescheduling.
  This capability serves as the last resort for any capacity related problems.

### Q: How to compare with Kubernetes federation?

They share the same goal of managing multiple clusters but this solution inherits the core idea of
virtualcluster such that providing a native kubernetes user experience to minimize the integration
cost for any existing applications. **Interestingly**, if there is only one tenant control plane, this
solution could be a direct replacement to KubeFed had it been used while no federated type CRDs
are needed.

