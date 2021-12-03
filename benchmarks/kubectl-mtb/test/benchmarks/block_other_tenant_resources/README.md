# Block access to other tenant resources <small>[MTB-PL1-CC-TI-2] </small>

**Profile Applicability:**

2

**Type:**

Configuration

**Category:**

Tenant Isolation

**Description:**

Access controls should be configured so that a tenant cannot view, edit, create, or delete namespaced resources belonging to another tenant.

**Rationale:**

Tenant resources should be isolated from other tenants.


**Audit:** 

Run the following commands to retrieve the list of namespaced resources available in Tenant B
```bash
kubectl --kubeconfig tenant-b api-resources --namespaced=true
```
For each namespaced resource, and each verb (get, list, create, update, patch, watch, delete, and deletecollection) issue the following command
```bash
kubectl --kubeconfig tenant-a -n b1 &lt;verb&gt; &lt;resource&gt;
```
Each command must return &#39;no&#39;



**namespaceRequired:** 

2

