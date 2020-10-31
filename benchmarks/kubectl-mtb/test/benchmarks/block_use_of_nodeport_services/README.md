# Block use of NodePort services <small>[MTB-PL1-BC-HI-1] </small>

**Profile Applicability:**

1

**Type:**

Behavioral Check

**Category:**

Host Isolation

**Description:**

Tenants should not be able to create services of type NodePort.

**Rationale:**

NodePorts configure host ports that cannot be secured using Kubernetes network policies and require upstream firewalls. Also, multiple tenants cannot use the same host port numbers.

**Audit:**

Create a deployment and an associated service exposing a NodePort. The service creation must fail.

**Remediation:**

Use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to block NodePort Services. You can use the policies present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies).


**namespaceRequired:** 

1

