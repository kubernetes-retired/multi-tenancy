# [MTB-PL1-BC-HI-4] Block use of host PID

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Host Isolation

**Description:**

Tenants should not be allowed to share the host process ID (PID) namespace.

**Rationale:**

The `hostPID` setting allows pods to share the host process ID namespace allowing potential privilege escalation. Tenant pods should not be allowed to share the host PID namespace.

**Audit:**

Create a pod or container that sets new `hostPID` to `true`. The pod creation must fail.

**Remediation:**

Define a `PodSecurityPolicy` with `hostPID` set to `false` and map the policy to each tenant's namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce that `hostPID` cannot be set to `true`.
