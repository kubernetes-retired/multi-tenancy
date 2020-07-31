# Create Network Policies <small>[MTB-PL2-BC-OPS-4] </small>

**Profile Applicability:**

2 <br>

**Type:**

Behavioral <br>

**Category:**

Self-Service Operations <br>

**Description:**

Tenants should be able to perform self-service operations by creating own network policies in their namespaces. <br>

**Rationale:**

Enables self-service management of network-policies. <br>

**Audit:**

Run the following commands to check for permissions to manage `network-policy` for each verb(get, create, update, patch, delete, and deletecollection) in the tenant namespace:
kubectl --kubeconfig=tenant-a -n a1 auth can-i &lt;verb&gt; networkpolicy
Each command must return &#39;yes&#39; <br>

 <br>



