# [MTB-PL2-BC-OPS-3] Create role bindings

**Profile Applicability:**

Level 2

**Type:**

Behavioral

**Category:**

Self-Service Operations

**Description:**

Tenants should be able to perform self-service operations by creating own roles and role-bindings in their namespaces by binding them together.

Tenants

**Rationale:**

Enables self-service management of roles/role-bindings to bind allowed roles/tenant users.

**Audit:**

Run the following commands to check for permissions to manage `rolebinding` and `role` for each verb(get, list, create, update, patch, watch, delete, and deletecollection) in the tenant namespace:

    kubectl --kubeconfig=tenant-a -n a1 auth can-i <verb> <resource>

Each command must return 'yes'

Create a `role` pod-reader with permission to view pods and create a `role binding` to this role.

    kubectl --kubeconfig=tenant-a -n a1 create role <role-name> --verb=get --resource=pods
    kubectl --kubeconfig=tenant-a -n a1 create rolebinding <rolebinding-name> --role=<role-name>
