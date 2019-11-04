# Block bind mounts

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Host Protection

**Description:**

Tenants should not be able to mount host volumes and folders (bind mounts).

**Rationale:**

The use of host volumes and directories can be used to access shared data or escalate priviliges
and also creates a tight coupling between a tenant workload and a host.

**Audit:**

Create a pod defining a volume of type hostpath. The pod creation must fail.

**Remediation:**

Define a `PodSecurityPolicy` or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce that a `hostPath` volume cannot be used.
