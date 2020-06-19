
<!DOCTYPE html>
<html>
  <head>
    <title>README</title>
  </head>
  <body>
  <h2> Block privileged containers [MTB-PL1-BC-CPI-5] </h2>
	<p>
		<b> Profile Applicability: </b> 1 <br>
		<b> Type: </b> Behavioral Check <br>
		<b> Category: </b> Control Plane Isolation <br>
		<b> Description: </b> By default a container is not allowed to access any devices on the host, but a “privileged” container can access all devices on the host. A process within a privileged container can also get unrestricted host access. Hence, tenants should not be allowed to run privileged containers. <br>
		<b> Remediation: </b> Define a `PodSecurityPolicy` with `privileged` set to `false` and map the policy to each tenant&#39;s namespace, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to prevent tenants from running privileged containers. <br>
	</p>
    
  </body>
</html>
