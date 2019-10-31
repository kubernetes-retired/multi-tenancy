# Ensure that users of Tenant A cannot use hostpaths

**Profile Applicability:**

Level 1

**Type:**

Behavioral

**Category:**

Host Protection

**Description:**

Tenant administrator should not be able to use volumes of type hostpath. 

**Rationale:**

When two use the same hostpath on the same node then they can affect each other as they are sharing the same file storage location and space.

**Audit:**

Create a pod defining a volume of type hostpath. The pod creation must fail.

**Remediation:**

Force pod to define a PodSecurityPolicy where the usage of hostpaths is disabled.
