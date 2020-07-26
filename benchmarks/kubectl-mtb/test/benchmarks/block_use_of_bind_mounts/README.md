# Block use of bind mounts <small>[MTB-PL1-BC-HI-1] </small>
**Profile Applicability:** 
1 <br>
**Type:** 
Behavioral Check <br>
**Category:** 
Host Protection <br>
**Description:** 
Tenants should not be able to mount host volumes and folders (bind mounts). <br>
**Remediation:**
Define a `PodSecurityPolicy` that restricts hostPath volumes and map the policy to each tenant namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce that a `hostPath` volume cannot be used. You can use the policies present [here](https://github.com/kubernetes-sigs/multi-tenancy/benchmarks/kubectl-mtb/test/policies). <br>

