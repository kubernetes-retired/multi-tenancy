# [MTB-PL2-BC-OPS-4] Create Network Policies

**Profile Applicability:**

Level 2

**Type:**

Behavioral

**Category:**

Self-Service Operations

**Description:**

Tenants should be able to perform self-service operations by creating own network policies in their namespaces.

Tenants

**Rationale:**

Enables self-service management of network-policies.

**Audit:**

Run the following commands to check for permissions to manage `network-policy` for each verb(get, list, create, update, patch, watch, delete, and deletecollection) in the tenant namespace:

    kubectl --kubeconfig=tenant-a -n a1 auth can-i <verb> networkpolicy

Each command must return 'yes'
