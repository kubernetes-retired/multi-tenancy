# Block use of host path volumes <small>[MTB-PL1-BC-HI-1] </small>

**Profile Applicability:**

1

**Type:**

Behavioral Check

**Category:**

Host Protection

**Description:**

Tenants should not be able to mount host volumes and directories

**Rationale:**

The use of host volumes and directories can be used to access shared data or escalate priviliges and also creates a tight coupling between a tenant workload and a host.

**Audit:**

Create a pod defining a volume of type hostpath. The pod creation must fail.

**Remediation:**

Define a `PodSecurityPolicy` that restricts hostPath volumes and map the policy to each tenant namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce that a `hostPath` volume cannot be used. You can use the policies present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies).


**namespaceRequired:** 

1

