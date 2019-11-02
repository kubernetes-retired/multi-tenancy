# Multi-tenancy-benchmarks

This repository contains a set of Multi-Tenancy Benchmarks mantained by the 
[Multi-Tenancy Working Group](https://github.com/kubernetes-sigs/multi-tenancy) that can be used to validate if a Kubernetes cluster 
is properly configured for multi-tenancy. A validation tool is also provided.

For background, see: [Multi-Tenancy Benchmarks Proposal](https://docs.google.com/document/d/1O-G8jEpiJxOeYx9Pd2OuOSb8859dTRNmgBC5gJv0krE/edit?usp=sharing).

## Documentation
- [Multi-Tenancy Profiles](documentation/definitions.md)
- [Benchmark Types](documentation/types.md)
- [Benchmark Categories](documentation/catagories.md)
- [Running the Multi-Tenancy Validation](documentation/run.md)
- [Roadmap](documentation/roadmap.md)
- [Contributing](documentation/contributing.md)

## Benchmarks

### Multi-Tenancy Profile Level 1
Items in this profile intend to:
* isolate and protect the kubernetes control plane from tenants
* use standard Kubernetes resources
* may inhibit select Kubernetes features. For example, a tenant may not be allowed to install a CRD


| Type              | Category                       | Benchmark                                          |
|-------------------|--------------------------------|----------------------------------------------------|
|     Behavioral    |  Control Plane Protection  |  [Ensure that a tenant cannot list cluster-wide resources](e2e/tests/tenantaccess)|
|     Behavioral    |  Tenant Protection  |  [Ensure that Tenant A cannot list namespaced resources from Tenant B](e2e/tests/tenantprotection)|
|     Configuration |  Fairness  |  [Ensure that Tenant A cannot starve other tenants from cluster wide resources](e2e/tests/resourcequotas)|
|     Behavioral    |  Tenant Isolation  |  [Ensure that users of Tenant A cannot modify Resource Quotas](e2e/tests/modify_resourcequotas)|
|     Behavioral    |  Tenant Isolation  |  [Ensure that users of Tenant A cannot modify resources managed by the cluster administrator](e2e/tests/modify_admin_resource/README.md)|
|     Behavioral    |  Network Protection & Isolation  |  [Ensure that users of Tenant A cannot connect to pods running in Tenant B](e2e/tests/network_isolation)|
|     Behavioral    |  Host Protection  |  [Ensure that users of Tenant A cannot use hostpaths](e2e/tests/deny_hostpaths)|
|     Behavioral    |  Host Protection  |  [Ensure that users of Tenant A cannot use NodePort](e2e/tests/deny_nodeports)|
|     Behavioral    |  Host Protection  |  [Ensure that users of Tenant A cannot use HostPort](e2e/tests/deny_hostports/README.md)|

### Multi-Tenancy Profile Level 2
This profile extends the "Level 1" profile. Items in this profile exhibit one or more of the following characteristics:
* may require multi-tenancy related CRDs or other Kubernetes extensions
* provides self-service creation of tenant namespaces
* provides self-service management of other namespace resources like network policies, roles, and role bindings

|  Type             |  Category                      | Check                                              |
|-------------------|--------------------------------|----------------------------------------------------|



### Multi-Tenancy Profile Level 3
This profile extends the "Level 2" profile. Items in this profile exhibit one or more of the following characteristics:
* are intended for environments or use cases where a stronger-level of multi-tenancy is paramount
* allows of all Kubernetes features. For example, a tenant can install their own CRD and different tenants may have different versions


|  Type             |  Category                      | Check                                              |
|-------------------|--------------------------------|----------------------------------------------------|
