# [MTB-PL3-BC-CPV-1] Create same CRD as different tenants

**Profile Applicability:**

Level 3

**Type:**

Behavioral

**Category:**

Control Plane Virtualization

**Description:**

Linux

**Rationale:**

Tenant Administrators should be able to create CRDs as different tenants in the cluster.

**Audit:**

Check the `RBAC` privileges of the tenant to create CRDs by running the following command.

    kubectl --kubeconfig=tenant-a auth can-i create crd

command must return `yes`

Create a CRD with the sample tenant manifest [file](https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenant.yaml), it should be created successfully and it must given an error with any change in any field/specs of the CRD.

**Remediation:**

Define a Custom Addmission Controller to validate the CRD manifest file, or use a policy engine such as [OPA/Gatekeeper](https://github.com/open-policy-agent/gatekeeper) or [Kyverno](https://kyverno.io) to validate against the default pattern.
