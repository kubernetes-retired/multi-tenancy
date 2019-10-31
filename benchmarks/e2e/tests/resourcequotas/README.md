# Ensure that users of Tenant A cannot modify Resource Quotas


**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Tenant Isolation

**Description:**

Users should not be able to modify the Resource Quotas defined in their Namespaces

**Rationale:**

Resource Quotas must be configured to guarantee isolation between tenants. Furthermore, it should be impossible for a tenant administrator to modify an existing Resource Quota.

**Audit:**

Run the following commands to retrieve the list of Resource Quotas configured in Tenant A:

  	kubectl --kubeconfig=tenant-a -n a1 ResourceQuota

For each Resource Quota returned run the following command:
	
	kubectl --kubeconfig=tenant-a -n a1 annotate ResourceQuota <resource-quota>  key1=value1 --dry-run

Each command must return 403 FORBIDDEN
