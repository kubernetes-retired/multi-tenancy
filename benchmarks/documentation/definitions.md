# Multi-Tenancy Profile Definitions

**Level 1** - Items in this profile intend to:
- isolate and protect the kubernetes control plane from tenants
- use standard Kubernetes resources
- may inhibit select Kubernetes features. For example, a tenant may not be allowed to install a CRD

**Level 2** - This profile extends the "Level 1" profile. Items in this profile exhibit one or more of the following characteristics:
- may require multi-tenancy related CRDs or other Kubernetes extensions
- provides self-service creation of tenant namespaces
- provides self-service management of other namespace resources like network policies, roles, and role bindings

**Level 3** - This profile extends the "Level 2" profile. Items in this profile exhibit one or more of the following characteristics:
- are intended for environments or use cases where a higher-level of multi-tenancy is paramount
- allows of all Kubernetes features. For example, a tenant can install their own CRD and different tenants may have different versions