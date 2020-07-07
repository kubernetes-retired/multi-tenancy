# Block use of NodePort services <small>[MTB-PL1-BC-HI-1] </small>
**Profile Applicability:** 
1
**Type:** 
Behavioral Check
**Category:** 
Host Isolation 
**Description:** 
Tenants should not be able to create services of type NodePort. 
**Remediation:**
Use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to block NodePort Services.

