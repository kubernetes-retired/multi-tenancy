# Ensure that users of Tenant A cannot modify resources managed by the cluster administrator

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Tenant Isolation

**Description:**

Tenant administrator should not be allowed to modify some of the namespaced resources created by the cluster administrator. This may include resources such as resource quotas, role bindings, limit ranges, service accounts, default network policy. 


**Rationale:**

Tenant administrator could impact other tenants if they could modify resources such as resource quotas or default network policy.

**Audit:**

The resources managed by the cluster administrator and that cannot be modified by tenant administrator are identified by the label configured in the benchmarks configuration YAML file. If no label is provided, then this test is skipped.
	
Run the following commands to retrieve the list of resources managed by the cluster administrator

  	kubectl --kubeconfig=tenant-a -n a1 get all -l <key>=<value>

For each returned by the first command verify that the resource cannot be modified by the tenant administrator:
	
	kubectl --dry-run --kubeconfig=tenant-a -n a1 annotate <resource-type> <resource-name>   key1=value1 

Each command must return 403 FORBIDDEN

Repeat these commands for ClusterRole and ClusterRoleBinding.
