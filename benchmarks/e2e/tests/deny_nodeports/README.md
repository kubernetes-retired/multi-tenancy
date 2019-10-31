# Ensure that users of Tenant A cannot use NodePort

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Host Protection

**Description:**

Tenant administrator should not be able to create services of type NodePort.


**Rationale:**

NodePorts are resources shared by all tenants. If a specific NodePort is used by a service in Tenant A, then the same service could not be defined in Tenant B. To avoid NodePort conflicts across tenants, NodePort usage must be forbidden.

**Audit:**

Create a deployment and an associated service exposing a NodePort. The service creation must fail.
