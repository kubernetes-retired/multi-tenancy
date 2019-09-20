# Tenant

This folder provides a set of CRDs used to manage tenant resources in a Kubernetes cluster.

## Overview
A cluster admin primarily leverages a tenant admin namespace to create, and thus manage all tenant related API objects including tenant namespaces (not directly though), arbitrary customized CRDs and policy objects like RBAC. This makes the tenant concept fully extensible instead of limiting the types of tenant objects specifically in TenantSpec. Basically, the tenant admin namespace name is specified in Tenant CR and tenant controller simply creates the namespace and configure the cluster RBAC rules for tenant admins to access various tenant related objects which are created on-demand (not implmented yet).

The most common tenant objects are tenant namespace CRs which encapsulate the configurations of tenant namespaces including name and RBAC rules etc. Tenant namespace controller does actual namespace creation for tenant users to use. The primary reason why we introduce tenant namespace CRD is that now tenant namespace creation can be RBAC permissioned, and tenant admins can do `kubectl get tenantnamespace -n <tenant admin namespace>` to list all the tenant namespaces. Otherwise had K8s namespace list API been used, all namespaces in the cluster would be exposed to tenant admins. Tenant admin can do self-service namespace creation via creating tenant namespace CR or even import existing namespace to tenant management system.

## Getting Started

### Install CRDs
```
kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/blob/master/tenant/config/crds/tenancy_v1alpha1_tenant.yaml
kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/blob/master/tenant/config/crds/tenancy_v1alpha1_tenantnamespace.yaml
```
### Install tenant-controller-managers
```
kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/blob/master/tenant/config/manager/all_in_one.yaml
```

## Basic Usage

Use the following cmd to create a tenant CR. A tenant admin namespace `tenant1admin` will be created.
```
kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/blob/master/tenant/config/samples/tenancy_v1alpha1_tenant.yaml
```

Use the following cmd to create a tenantnamespace CR. A tenant namespace `t1-ns1` will be created.
```
kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/blob/master/tenant/config/samples/tenancy_v1alpha1_tenantnamespace.yaml
```
