# Block privileged containers <small>[MTB-PL1-BC-CPI-5] </small>

**Profile Applicability:**

1

**Type:**

Behavioral Check

**Category:**

Control Plane Isolation

**Description:**

Linux

**Rationale:**

By default a container is not allowed to access any devices on the host, but a “privileged” container can access all devices on the host. A process within a privileged container can also get unrestricted host access. Hence, tenants should not be allowed to run privileged containers.

**Audit:**

Create a pod or container that sets `privileged` to `true` in its `securityContext`. The pod creation must fail.

**Remediation:**

Define a `PodSecurityPolicy` with `privileged` set to `false` and map the policy to each tenant&#39;s namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to prevent tenants from running privileged containers. You can use the policies present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies).


**namespaceRequired:** 

1

