# Block use of host IPC <small>[MTB-PL1-BC-HI-5] </small>
**Profile Applicability:** 
1
**Type:** 
Behavioral Check
**Category:** 
Host Isolation 
**Description:** 
Tenants should not be allowed to share the host&#39;s inter-process communication (IPC) namespace. 
**Remediation:**
Define a `PodSecurityPolicy` with `hostIPC` set to `false` and map the policy to each tenant&#39;s namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to enforce that `hostPID` cannot be set to `true`.

