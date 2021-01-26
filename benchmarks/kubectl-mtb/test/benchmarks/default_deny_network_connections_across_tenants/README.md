# Default deny network connections across tenants <small>[MTB-PL1-BC-NI-1] </small>

**Profile Applicability:**

2

**Type:**

Behavioral

**Category:**

Network Isolation

**Description:**

By default, Kubernetes allows network connections across all pods and services in the same cluster. In a multi-tenant configuration a tenant should not be allowed to connect to pods and services belonging to another tenant, unless the connections are explicitly allowed.

**Rationale:**

Tenants should have explicit control over ingress connections for their workloads.

**Audit:**

Create a pod in a tenant namespace. Then create services which export ranges of ports. Connections to these services from other tenants should fail.


**namespaceRequired:** 

2

