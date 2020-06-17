<p>id: MTB-PL1-BC-CPI-5
title: Block privileged containers
benchmarkType: Behavioral Check
category: Control Plane Isolation
description: By default a container is not allowed to access any devices on the host, but a “privileged” container can access all devices on the host. A process within a privileged container can also get unrestricted host access. Hence, tenants should not be allowed to run privileged containers.
remediation: Define a <code>PodSecurityPolicy</code> with <code>privileged</code> set to <code>false</code> and map the policy to each tenant&rsquo;s namespace, or use a policy engine such as <a href="https://github.com/open-policy-agent/gatekeeper">OPA/Gatekeeper</a> or <a href="https://kyverno.io">Kyverno</a> to prevent tenants from running privileged containers.
profileLevel: 1</p>
