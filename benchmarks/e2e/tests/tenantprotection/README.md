## Ensure that Tenant A cannot list namespaced resources from Tenant B

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Tenant Protection

**Description:**

Tenants should not be able to interact with any of the namespaced resources belonging to another Tenant

**Rationale:**

Access controls should be configured for tenants so that one tenant cannot list, create, modify or delete namespaced resources in another tenant.

**Audit:**

Run the following commands to retrieve the list of namespaced resources available in Tenant B

  	kubectl --kubeconfig cluster-admin api-resources --namespaced=true

For each namespaced resource, issue the following command
	
	kubectl --kubeconfig tenant-a -n b1 get <resource>

Each command must return 403 FORBIDDEN
