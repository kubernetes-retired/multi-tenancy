# [MTB-PL1-BC-CPI-6] Block privilege escalation

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Control Plane Isolation

**Description:**

Linux 

**Rationale:**

The `securityContext.allowPrivilegeEscalation` setting allows a process to gain more privileges from its parent process. Processes in tenant containers should not be allowed to gain additional priviliges.

**Audit:**

Create a pod or container that sets `allowPrivilegeEscalation` to `true` in its `securityContext`. The pod creation must fail.

**Remediation:**

Define a `PodSecurityPolicy` with `allowPrivilegeEscalation` set to `false` and map the policy to each tenant's namespace,  or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to prevent privilege escalation.
