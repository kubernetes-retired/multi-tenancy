# Block role privilege escalation <small>[MTB-PL2-CC-TI-1] </small>

**Profile Applicability:**

2

**Type:**

Configuration

**Category:**

Tenant Isolation

**Description:**

Tenants should not have the ability to escalate their Role beyond the permissions the administrator gives them.


**Audit:**

`kubectl auth can-i escalate role --as tenant -n namespace` and `kubectl auth can-i bind clusterrole/cluster-admin --as tenant -n namespace` should return &#34;no&#34; for each tenant.


**Remediation:**

Ensure that users can&#39;t perform the &#34;escalate&#34; verb on Roles. Ensure users can&#39;t perform the &#34;bind&#34; verb on arbitrary Roles/ClusterRoles. Ref: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#privilege-escalation-prevention-and-bootstrapping


