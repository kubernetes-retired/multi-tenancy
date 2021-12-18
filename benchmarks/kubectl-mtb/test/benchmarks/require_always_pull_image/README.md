# Require always imagePullPolicy <small>[MTB-PL1-CC-DI-1] </small>

**Profile Applicability:**

1

**Type:**

Configuration Check

**Category:**

Data Isolation

**Description:**

Require that the image pull policy is always set to to `Always` so that the users an be assured that their private images can only be used by those who have the credentials to pull them.

**Remediation:**

Enable the AlwaysPullImages admission plugin in the kube-apiserver or create dynamic admission controller that enforces/mutates the `imagePullPolicy` to be `Always` for all Pods in the cluster.


**namespaceRequired:** 

1

