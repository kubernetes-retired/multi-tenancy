# Configure namespace resource quotas <small>[MTB-PL1-CC-FNS-1] </small>
**Profile Applicability:** 
1 <br>
**Type:** 
Configuration <br>
**Category:** 
Fairness <br>
**Description:** 
Namespace resource quotas should be used to allocate, track, and limit a tenant&#39;s use of shared resources. <br>
**Remediation:**
Create ResourceQuota object, you can use the configuration file present in `quotas` directory, example `kubectl apply -f test/quotas/ns_quota.yaml` <br>

