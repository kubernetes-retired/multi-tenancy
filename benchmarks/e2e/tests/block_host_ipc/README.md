# [MTB-PL1-BC-HI-5] Block use of host IPC

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Host Isolation

**Description:**

Tenants should not be allowed to share the host's inter-process communication (IPC) namespace.

**Rationale:**

The `hostIPC` setting allows pods to share the host's inter-process communication (IPC) namespace allowing potential access to host processes or processes belonging to other tenants.

**Audit:**

Create a pod or container that sets new `hostIPC` to `true`. The pod creation must fail.

**Remediation:**

Define a `PodSecurityPolicy` with `hostIPC` set to `false` and map the policy to each tenant's namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce that `hostPID` cannot be set to `true`.
