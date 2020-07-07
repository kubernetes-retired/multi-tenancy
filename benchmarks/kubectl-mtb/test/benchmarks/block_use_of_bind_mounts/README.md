# Block use of bind mounts <small>[MTB-PL1-BC-HI-1] </small>
**Profile Applicability:** 
1
**Type:** 
Behavioral Check
**Category:** 
Host Protection 
**Description:** 
Tenants should not be able to mount host volumes and folders (bind mounts). 
**Remediation:**
Define a `PodSecurityPolicy` that restricts hostPath volumes and map the policy to each tenant namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce that a `hostPath` volume cannot be used.

