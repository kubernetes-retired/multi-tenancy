# Block privileged containers <small>MTB-PL1-BC-CPI-5</small>

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Control Plane Isolation

**Description:**

Linux 

**Rationale:**

Privileged containers are defined as any container where the container uid 0 is mapped to the host’s uid 0. . By default a container is not allowed to access any devices on the host, but a “privileged” container is given access to all devices on the host. A process within a privileged container can get unrestricted host access. Additionally, with `securityContext.allowPrivilegeEscalation` enabled, a process can gain privileges from its parent process.

**Audit:**

Create a pod or container that sets `privileged` to `true` in its `securityContext`. The pod creation must fail. Additionally, create a pod or container that sets `allowPrivilegeEscalation`. The pod creation must also fail.

**Remediation:**

Define a `PodSecurityPolicy` with `privileged` set to `false` and `allowPrivilegeEscalation` set to `false` or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to prevent privileged containers and privilege escalation.
