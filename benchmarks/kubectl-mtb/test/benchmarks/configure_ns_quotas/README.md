# Configure namespace resource quotas <small>[MTB-PL1-CC-FNS-1] </small>

**Profile Applicability:**

1 <br>

**Type:**

Configuration <br>

**Category:**

Fairness <br>

**Description:**

Namespace resource quotas should be used to allocate, track, and limit a tenant&#39;s use of shared resources. <br>

**Rationale:**

Resource quotas must be configured for each tenant namespace, to guarantee isolation and fairness across tenants. <br>

**Audit:**

Run the following command to show configured quotas. Make sure that a quota is configured for CPU, memory, and storage resources.
kubectl --kubeconfig=tenant-a -n a1 describe quota <br>

Create ResourceQuota object, you can use the configuration file present in `quotas` directory, example `kubectl apply -f test/quotas/ns_quota.yaml` <br>



