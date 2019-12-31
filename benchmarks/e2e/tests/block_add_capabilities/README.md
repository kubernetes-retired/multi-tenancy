# Block add capabilities <small>MTB-PL1-BC-CPI-3</small>

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Control Plane Isolation

**Description:**

Linux 

**Rationale:**

Linux allows defining fine-grained permissions using capabilities. With Kubernetes, it is possible to add capabilities for pods that escalate the level of kernel access and allow other potentially dangerous behaviors.

**Audit:**

Create a pod or container that adds new `capabilities` in its `securityContext`. The pod creation must fail.

**Remediation:**

Define a `PodSecurityPolicy` with `allowedCapabilities` or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce new capabilities cannot be added.
