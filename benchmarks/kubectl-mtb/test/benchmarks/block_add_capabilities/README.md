# Block add capabilities <small>[MTB-PL1-BC-CPI-3] </small>

**Profile Applicability:**

1 <br>

**Type:**

Behavioral Check <br>

**Category:**

Control Plane Isolation <br>

**Description:**

Linux <br>

**Rationale:**

Linux allows defining fine-grained permissions using capabilities. With Kubernetes, it is possible to add capabilities for pods that escalate the level of kernel access and allow other potentially dangerous behaviors. <br>

**Audit:**

Create a pod or container that adds new `capabilities` in its `securityContext`. The pod creation must fail. <br>

Define a `PodSecurityPolicy` with `allowedCapabilities` and map the policy to each tenant namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce new capabilities cannot be added. You can use the policies present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies). <br>



