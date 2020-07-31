# Block privilege escalation <small>[MTB-PL1-BC-CPI-6] </small>

**Profile Applicability:**

1 <br>

**Type:**

Behavioral Check <br>

**Category:**

Control Plane Isolation <br>

**Description:**

The `securityContext.allowPrivilegeEscalation` setting allows a process to gain more privileges from its parent process. Processes in tenant containers should not be allowed to gain additional priviliges. <br>

**Rationale:**

The `securityContext.allowPrivilegeEscalation` setting allows a process to gain more privileges from its parent process. Processes in tenant containers should not be allowed to gain additional priviliges. <br>

**Audit:**

Create a pod or container that sets `allowPrivilegeEscalation` to `true` in its `securityContext`. The pod creation must fail. <br>

Define a `PodSecurityPolicy` with `allowPrivilegeEscalation` set to `false` and map the policy to each tenant&#39;s namespace,  or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to prevent privilege escalation. You can use the policies present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies). <br>



