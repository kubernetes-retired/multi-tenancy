# Tenant

This folder provides a set of CRDs used to manage tenant resources in a Kubernetes cluster.

## Overview
A cluster admin primarily leverages a tenant admin namespace to create, and thus manage all tenant related API objects including tenant namespaces (not directly though), arbitrary customized CRDs and policy objects like RBAC. This makes the tenant concept fully extensible instead of limiting the types of tenant objects specifically in TenantSpec. Basically, the tenant admin namespace name is specified in Tenant CR and tenant controller simply creates the namespace and configure the cluster RBAC rules for tenant admins to access various tenant related objects which are created on-demand (not implmented yet).

The most common tenant objects are tenant namespace CRs which encapsulate the configurations of tenant namespaces including name and RBAC rules etc. Tenant namespace controller does actual namespace creation for tenant users to use. The primary reason why we introduce tenant namespace CRD is that now tenant namespace creation can be RBAC permissioned, and tenant admins can do `kubectl get tenantnamespace -n <tenant admin namespace>` to list all the tenant namespaces. Otherwise had K8s namespace list API been used, all namespaces in the cluster would be exposed to tenant admins. Tenant admin can do self-service namespace creation via creating tenant namespace CR or even import existing namespace to tenant management system.

## Getting Started

### Install CRDs
```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenant.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenantnamespace.yaml
```
### Install tenant-controller-managers
```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/manager/all_in_one.yaml
```

## Basic Usage

As a cluster admin, use the following cmd to create a tenant CR. A tenant admin namespace `tenant1admin` will be created.
```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/samples/tenancy_v1alpha1_tenant.yaml
```

Create a testing service account, say `t1-admin1` in any namespace, to mimic a tenant admin. (This [script]( https://gist.github.com/innovia/fbba8259042f71db98ea8d4ad19bd708) helps to create SA with corresponding KubeConfig file)

Then you can edit the tenant CR and add `t1-admin1` to the tenantAdmins list
```
kubectl edit tenant tenant-sample
```

```
apiVersion: tenancy.x-k8s.io/v1alpha1
kind: Tenant
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: tenant-sample
spec:
  # Add fields here
  tenantAdminNamespaceName: "tenant1admin"
  tenantAdmins:
    - kind: ServiceAccount
      name: t1-admin1
      namespace: XXXXX
``` 

Once `t1-admin1` is added, tenant controller will generate the following RBAC settings:
* `tenant-sample-tenant-admin-role` cluster role and `tenant-sample-tenant-admins-rolebinding` cluster rolebindings, which allow `t1-admin1` to access `tenant-sample` CR and `tenant1admin` namespace.
* `tenant-admin-role` role and `tenant-admins-rolebinding` rolebinding in `tenant1admin` namespace, which allow `t1-admin1` to create/update/delete tenantnamespace CR in `tenant1admin` namespace. 

`t1-admin1` can do self service namespace creation by creating a tenantnamespace CR in `tenant1admin` namespace:
```
KUBECONFIG=$PATH_TO_T1_ADMIN1_KUBECONFIG kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/samples/tenancy_v1alpha1_tenantnamespace.yaml
```
Tenantnamespace controller will create a `t1-ns1` namespace and update `tenant-sample-tenant-admin-role` cluster role to allow all cluster admins to access `t1-ns1` namespace.


