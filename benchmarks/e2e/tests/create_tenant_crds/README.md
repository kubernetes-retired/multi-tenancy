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

Create a tenant in your cluster by creating the CRDs following  [steps](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/tenant#getting-started), Tenant should be created successfully and it must given an error with any change in any field/specs of the CRD.

**Remediation:**

Use [VirtualCluster](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/incubator/virtualcluster) to enable Control Plane Virtualization in you K8s cluster and creation of same CRD as different tenants.
