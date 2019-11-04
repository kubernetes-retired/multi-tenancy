# Configure namespace resource quotas 

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Fairness

**Description:**

Namespace resource quotas should be used to allocate, track, and limit a tenant's use of shared resources.

Tenants 

**Rationale:**

Resource quotas must be configured for each tenant namespace, to guarantee isolation and fairness across tenants. 

**Audit:**

Run the following commands to retrieve the list of Resource Quotas configured in Tenant A:

  	kubectl --kubeconfig=tenant-a -n a1 ResourceQuota

For each Resource Quota returned run the following command:
	
	kubectl --kubeconfig=tenant-a -n a1 annotate ResourceQuota <resource-quota>  key1=value1 --dry-run

Each command must return 403 FORBIDDEN
