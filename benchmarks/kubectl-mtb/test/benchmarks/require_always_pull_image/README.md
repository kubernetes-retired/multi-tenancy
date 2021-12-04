# Require always imagePullPolicy <small>[MTB-PL1-CC-DI-1] </small>

**Profile Applicability:**

1

**Type:**

Configuration Check

**Category:**

Data Isolation

**Description:**

Set the image pull policy to `Always` so that the users an be assured that their private images can only be used by those who have the credentials to pull them.

**Remediation:**

Set `imagePullPolicy` to `always` for the container or enable the AlwaysPullImages admission plugin.


**namespaceRequired:** 

1

