## Block access to other tenant resources

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Tenant Protection

**Description:**

Access controls should be configured so that a tenant cannot view, edit, create, or delete namespaced resources belonging to another tenant.

**Rationale:**

Tenant resources should be isolated from other tenants.

**Audit:**

Run the following commands to retrieve the list of namespaced resources available in Tenant B

  	kubectl --kubeconfig cluster-admin api-resources --namespaced=true

For each namespaced resource, issue the following command
	
	kubectl --kubeconfig tenant-a -n b1 get <resource>

Each command must return 403 FORBIDDEN
