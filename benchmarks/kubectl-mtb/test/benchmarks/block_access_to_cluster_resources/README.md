# Block access to cluster resources <small>[MTB-PL1-CC-CPI-1] </small>

**Profile Applicability:**

1 <br>

**Type:**

Configuration Check <br>

**Category:**

Control Plane Isolation <br>

**Description:**

Tenants should not be able to view, edit, create, or delete cluster (non-namespaced) resources such Node, ClusterRole, ClusterRoleBinding, etc. <br>

**Rationale:**

Access controls should be configured for tenants so that a tenant cannot list, create, modify or delete cluster resources <br>

**Audit:**

Run the following commands to retrieve the list of non-namespaced resources
kubectl --kubeconfig cluster-admin api-resources --namespaced=false
For all non-namespaced resources, and each verb (get, list, create, update, patch, watch, delete, and deletecollection) issue the following commands:
kubectl --kubeconfig tenant-a auth can-i &lt;verb&gt; &lt;resource&gt;
Each command must return &#39;no&#39; <br>

 <br>



