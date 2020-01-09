# [MTB-PL1-BC-CPI-4] Require run as non-root user

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Control Plane Isolation

**Description:**

Linux 

**Rationale:**

Pprocesses in containers run as the root user (uid 0), by default. To prevent potential compromise of container hosts, specify a least privileged user ID when building the container image and require that application containers run as non root users.

**Audit:**

Create a pod or container that does not set `runAsNonRoot` to `true` in its `securityContext`. The pod creation must fail for both cases.

**Remediation:**

Define a `PodSecurityPolicy` a `runAsUser` rule set to `MustRunAsNonRoot` and map the policy to each tenant's namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce that `runAsNonRoot` is set to `true` for tenant pods.
