# [MTB-PL1-BC-CPI-5] Block privileged containers

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Control Plane Isolation

**Description:**

Linux 

**Rationale:**

By default a container is not allowed to access any devices on the host, but a “privileged” container can access all devices on the host. A process within a privileged container can also get unrestricted host access. Hence, tenants should not be allowed to run privileged containers.

**Audit:**

Create a pod or container that sets `privileged` to `true` in its `securityContext`. The pod creation must fail.

**Remediation:**

Define a `PodSecurityPolicy` with `privileged` set to `false` and map the policy to each tenant's namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to prevent tenants from running privileged containers.
