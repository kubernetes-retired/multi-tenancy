# Ensure that users of Tenant A cannot use HostPort

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Host Protection

**Description:**

Tenant administrator should not be able to create pods with containers using host ports.

**Rationale:**

Host ports are resources shared by all tenants. If a specific host port is used by a Pod running in Tenant A, then the same host port  could not be used  by Tenant B on the same node. To avoid host port conflicts across tenants, host port usage must be forbidden.

**Audit:**

Create a pod defining a container using a host port. The pod creation must fail.
