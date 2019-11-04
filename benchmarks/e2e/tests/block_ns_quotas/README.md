# Block Resource Quotas

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Tenant Isolation

**Description:**

Tenants should not be able to modify the resource quotas defined in their namespaces

**Rationale:**

Resource Quotas must be configured to guarantee isolation between tenants. Furthermore, it should not be impossible for a tenant administrator to modify an existing resource quota as they may over-allocate resources and impact other tenants.

**Audit:**

Run the following commands to retrieve the list of Resource Quotas configured in Tenant A:

  	kubectl --kubeconfig=tenant-a -n a1 ResourceQuota

For each Resource Quota returned run the following command:
	
	kubectl --kubeconfig=tenant-a -n a1 annotate ResourceQuota <resource-quota>  key1=value1 --dry-run

Each command must return 403 FORBIDDEN
