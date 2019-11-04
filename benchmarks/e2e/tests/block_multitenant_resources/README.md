# Block modification of multi-tenancy resources

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Tenant Isolation

**Description:**

Each tenant namespace may contain resources setup by the cluster administrator for multi-tenancy, such as role bindings, resource quotas, and network policies.

Tenants should not be allowed to modify the namespaced resources created by the cluster administrator for multi-tenancy.


**Rationale:**

Tenants can escalate priviliges and impact other tenants if they are able to delete or modify multi-tenancy resources such as namespace resource quotas or default network policy.

**Audit:**

The resources managed by the cluster administrator and that cannot be modified by tenant administrator are identified by the label configured in the benchmarks configuration YAML file. If no label is provided, then this test is skipped (resource quotas are covered by a separate test.)
	
Run the following commands to retrieve the list of resources managed by the cluster administrator

  	kubectl --kubeconfig=tenant-a -n a1 get all -l <key>=<value>

For each returned by the first command verify that the resource cannot be modified by the tenant administrator:
	
	kubectl --dry-run --kubeconfig=tenant-a -n a1 annotate <resource-type> <resource-name>   key1=value1 

Each command must return 403 FORBIDDEN

Repeat these commands for ClusterRole and ClusterRoleBinding.
