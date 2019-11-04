# Block NodePort services

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Host Protection

**Description:**

Tenants should not be able to create services of type NodePort.

**Rationale:**

NodePorts configure host ports that cannot be secured using Kubernetes network policies and require upstream firewalls. Also, multiple tenants cannot use the same host port numbers.

**Audit:**

Create a deployment and an associated service exposing a NodePort. The service creation must fail.
