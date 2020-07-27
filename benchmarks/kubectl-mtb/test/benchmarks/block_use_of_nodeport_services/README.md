# Block use of NodePort services <small>[MTB-PL1-BC-HI-1] </small>
**Profile Applicability:** 
1 <br>
**Type:** 
Behavioral Check <br>
**Category:** 
Host Isolation <br>
**Description:** 
Tenants should not be able to create services of type NodePort. <br>
**Remediation:**
Use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to block NodePort Services. You can use the policies present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies). <br>

