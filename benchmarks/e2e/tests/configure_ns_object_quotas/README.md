# [MTB-PL1-CC-FNS-2]Configure namespace object limits

**Profile Applicability:**

Level 1

**Type:**

Configuration

**Category:**

Fairness

**Description:**

Namespace resource quotas should be used to allocate, track and limit the number of objects, of a particular type, that can be created within a namespace.

**Rationale:**

Resource quotas must be configured for each tenant namespace, to guarantee fair number of objects across tenants.

**Audit:**

Run the following command to show configured quotas. Make sure that a quota is configured for API objects(PersistentVolumeClaim, LoadBalancer, NodePort ,Pods etc).

`kubectl --kubeconfig=tenant-a -n a1 describe resourcequota`
