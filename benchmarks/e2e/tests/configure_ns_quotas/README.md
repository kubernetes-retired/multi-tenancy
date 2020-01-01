# [MTB-PL1-CC-FNS-1] Configure namespace resource quotas 

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

Run the following command to show configured quotas. Make sure that a quota is configured for CPU, memory, and storage resources.

    kubectl --kubeconfig=tenant-a -n a1 describe quota
