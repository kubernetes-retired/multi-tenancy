# Create Network Policies <small>[MTB-PL2-BC-OPS-4] </small>

**Profile Applicability:**

2

**Type:**

Behavioral

**Category:**

Self-Service Operations

**Description:**

Tenants should be able to perform self-service operations by creating own network policies in their namespaces.

**Rationale:**

Enables self-service management of network-policies.

**Audit:**

Run the following commands to check for permissions to manage `network-policy` for each verb(get, create, update, patch, delete, and deletecollection) in the tenant namespace:
```bash
kubectl --kubeconfig=tenant-a -n a1 auth can-i verb networkpolicy
```
Each command must return &#39;yes&#39;



**namespaceRequired:** 

1

