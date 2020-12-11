# Block modification of resource quotas <small>[MTB-PL1-CC-TI-1] </small>

**Profile Applicability:**

1

**Type:**

Behavioral Check

**Category:**

Tenant Isolation

**Description:**

Tenants should not be able to modify the resource quotas defined in their namespaces

**Rationale:**

Resource quotas must be configured for isolation and fairness between tenants. Tenants should not be able to modify existing resource quotas as they may exhaust cluster resources and impact other tenants.

**Audit:**

Run the following commands to check for permissions to manage quotas in the tenant namespace:
```shell
kubectl --kubeconfig=tenant-a -n a1 auth can-i create quota
kubectl --kubeconfig=tenant-a -n a1 auth can-i update quota
kubectl --kubeconfig=tenant-a -n a1 auth can-i patch quota
kubectl --kubeconfig=tenant-a -n a1 auth can-i delete quota
kubectl --kubeconfig=tenant-a -n a1 auth can-i deletecollection quota
```
Each command must return &#39;no&#39;&#34;


**namespaceRequired:** 

1

