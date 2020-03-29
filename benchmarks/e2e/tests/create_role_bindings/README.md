# [MTB-PL2-BC-OPS-3] Create role bindings

**Profile Applicability:**

Level 2

**Type:**

Behavioral

**Category:**

Self-Service Operations

**Description:**

Tenants should be able to do self service by creating own roles and role-bindings in their namespaces by binding them together.

Tenants

**Rationale:**

Enables self-service management of roles/role-bindings to bind allowed roles/tenant users.

**Audit:**

Run the following commands to check for permissions to manage `rolebinding` and `role` for each verb(get, list, create, update, patch, watch, delete, and deletecollection) in the tenant namespace:

    kubectl --kubeconfig=tenant-a -n a1 auth can-i <verb> <resource>

Each command must return 'yes'

Create a `role` and `rolebinding` in the tenant namespace as tenant-administrator for the `namespaced resources` and further bind the role with the help of newly created rolebinding. It should be a success.
