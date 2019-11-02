# Multi-Tenancy Concepts

## Tenant

A tenant is a user, or user group, with access to a set of namespaces that are isolated from namespaces owned by other tenants.

# Multi-Tenancy Roles

## Cluster Administrator

A cluster administrator has access to all namespaces and can configure new tenant namespaces. When creating a new tenant namespace, the cluster administrator would typically configure resources such as resource quotas, limit ranges, roles, role bindings, service accounts or default network policies. This may be an automated or a manual process.

## Tenant Administrator

A tenant administrator has access to and manages a set of namespaces. The tenant administrator may be able to manage allowed multi-tenancy related resources, such as adding new role bindings, service accounts, or network policies. As an example, if a namespace hierarchy is used, the tenant administrator will be responsible for managing the hierarchy.

## Tenant User (Optional)

A Tenant Administrator may optionally define additional roles for the namespaces they manage. The scope of the multi tenancy benchmarks is to test the tenant isolation from the point of view of the tenant administrators - having additional users is not required.

# Multi-Tenancy Profiles

## Level 1
Items in this profile intend to:
- isolate and protect the kubernetes control plane from tenants
- use standard Kubernetes resources
- may inhibit select Kubernetes features. For example, a tenant may not be allowed to install a CRD

## Level 2
This profile extends the "Level 1" profile. Items in this profile exhibit one or more of the following characteristics:
- may require multi-tenancy related CRDs or other Kubernetes extensions
- provides self-service creation of tenant namespaces
- provides self-service management of other namespace resources like network policies, roles, and role bindings

## Level 3
This profile extends the "Level 2" profile. Items in this profile exhibit one or more of the following characteristics:
- are intended for environments or use cases where a higher-level of multi-tenancy is paramount
- allows of all Kubernetes features. For example, a tenant can install their own CRD and different tenants may have different versions