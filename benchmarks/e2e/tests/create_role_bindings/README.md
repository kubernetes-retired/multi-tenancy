# [MTB-PL2-BC-OPS-3] Create role bindings

**Profile Applicability:**

Level 2

**Type:**

Behavioral

**Category:**

Self-Service Operations

**Description:**

Tenants should be able to do self service by creating own role-bindings in their namespaces.

Tenants

**Rationale:**

Enables self-service management of role-bindings to bind allowed roles/tenant users.

**Audit:**

Run the following commands to check for permissions to manage role-bindings in the tenant namespace:

    kubectl --kubeconfig=tenant-a -n a1 auth can-i create rolebinding
    kubectl --kubeconfig=tenant-a -n a1 auth can-i update rolebinding
    kubectl --kubeconfig=tenant-a -n a1 auth can-i patch rolebinding
    kubectl --kubeconfig=tenant-a -n a1 auth can-i delete rolebinding
    kubectl --kubeconfig=tenant-a -n a1 auth can-i deletecollection rolebinding

Each command must return 'yes'
