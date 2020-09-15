# Multi-Tenancy Concepts

## Tenant

A tenant is a user, or a group of users, that owns a set of namespaces that are isolated from namespaces owned by other tenants.

# Multi-Tenancy Roles

## Cluster Administrator

A cluster administrator has access to all cluster resources and can configure new tenant namespaces. When creating a new tenant namespace, the cluster administrator can configure multi-tenancy control resources such as resource quotas, limit ranges, roles, role bindings, service accounts or default network policies. This can be an automated or a manual process.

## Tenant Administrator

A tenant administrator manages namespaces that belong to the tenant. When self-service namespace provisionning is enabled, the tenant administrator can create new namespaces. The tenant adminsitartor may also be able to manage some multi-tenancy control resources e.g. adding new role bindings, service accounts, or network policies. If a namespace hierarchy is used, the tenant administrator is responsible for managing the hierarchy.

## Tenant User (Optional)

A tenant administrator can define new roles and role-bindings for their namespaces. The scope of the multi tenancy benchmarks is to test the tenant isolation from the point of view of the tenant administrators - having additional tenant user roles is not required.

# Multi-Tenancy Profiles

## Level 1
Items in this profile:
- isolate and protect the kubernetes control plane from tenants
- use standard Kubernetes resources
- may inhibit cluster-wide Kubernetes features. For example, a tenant may not be allowed to install a CRD

## Level 2
This profile extends the "Level 1" profile. Items in this profile:
- may require multi-tenancy related CRDs or other Kubernetes extensions
- enable self-service creation of tenant namespaces
- enable self-service management of other namespace resources like network policies, roles, and role bindings

<br/><br/>
*Read Next >> [Benchmark Types](types.md)*
