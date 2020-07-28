# Require run as non-root user <small>[MTB-PL1-BC-CPI-4] </small>
**Profile Applicability:** 
1 <br>
**Type:** 
Behavioral Check <br>
**Category:** 
Control Plane Isolation <br>
**Description:** 
Processes in containers run as the root user (uid 0), by default. To prevent potential compromise of container hosts, specify a least privileged user ID when building the container image and require that application containers run as non root users. <br>
**Remediation:**
Define a PodSecurityPolicy a runAsUser rule set to MustRunAsNonRoot and map the policy to each tenant&#39;s namespace, or use a policy engine such as OPA/Gatekeeper or Kyverno to enforce that runAsNonRoot is set to true for tenant pods. You can use the policies present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies). <br>

