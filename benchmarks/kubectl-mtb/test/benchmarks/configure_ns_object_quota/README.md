# Configure namespace object limits <small>[MTB-PL1-CC-FNS-2] </small>
**Profile Applicability:** 
1 <br>
**Type:** 
Configuration <br>
**Category:** 
Fairness <br>
**Description:** 
Namespace resource quotas should be used to allocate, track and limit the number of objects, of a particular type, that can be created within a namespace. <br>
**Remediation:**
Create ResourceQuota object, you can use the configuration file present in `quotas` directory, example `kubectl apply -f test/quotas/ns_quota.yaml` <br>

**Audit:** 
Run the following command to show configured quotas. Make sure that a quota is configured for API objects(PersistentVolumeClaim, LoadBalancer, NodePort ,Pods etc).
kubectl --kubeconfig=tenant-a -n a1 describe resourcequota <br>

**Rationale:** 
Resource quotas must be configured for each tenant namespace, to guarantee fair number of objects across tenants. <br>

