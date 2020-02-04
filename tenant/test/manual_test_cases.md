# **Manual Test Cases**

## Requirements

#### First Install CRDs
```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenant.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenantnamespace.yaml
```
verify using `kubectl get crd`
output :
```
NAME                                CREATED AT
tenantnamespaces.tenancy.x-k8s.io   2020-02-04T16:30:51Z
tenants.tenancy.x-k8s.io            2020-02-04T16:30:50Z
```

### Install tenant-controller-managers
```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/manager/all_in_one.yaml
```
This will create a `tenant-system` namespace. Verify it using `kubectl get namespace`
 
### Tenants 
``` 
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/test/config/tenant/tenant-a.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/test/config/tenant/tenant-b.yaml
```
verify using `kubectl get ns`, 2 new namespaces `tenantarootns` and `tenantbrootns` are created

### Service Accounts
```
bash https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/test/create_service_account_with_kubeconfig.sh tenant-a-admin default
bash https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/test/create_service_account_with_kubeconfig.sh tenant-b-admin default
```

kubeconfig file path is `/tmp/kube/`


## Test Cases

### Use Case 1: When `tenant-a-admin` is admin of `tenant-a`

``` 
kubectl edit tenant tenant-a
```

``` 
apiVersion: tenancy.x-k8s.io/v1alpha1
kind: Tenant
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: tenant-a
spec:
  # Add fields here
  tenantAdminNamespaceName: "tenantarootns"
  tenantAdmins:
    - kind: ServiceAccount
      name: tenant-a-admin
      namespace: default
```

### Test Case 1 : 
`tenant-a-admin` will be able to create `tenantnamespace-a` in `tenantarootns`

```
KUBECONFIG=/tmp/kube/k8s-tenant-a-admin-default-conf kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/test/tenantnamespace/tenantnamespace-a.yaml
```

### Output 1:
```tenantnamespace.tenancy.x-k8s.io/tenantnamespace-a created```

Verify using `kubectl get tenantnamespace -n tenantarootns`

##

### Test Case 2 : 
`tenant-b-admin` will not be able to create `tenantnamespace-a` in `tenantarootns`

```
KUBECONFIG=/tmp/kube/k8s-tenant-b-admin-tenant-system-conf kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/test/tenantnamespace/tenantnamespace-a.yaml
```

### Output 2:
```
Error from server (Forbidden): error when retrieving current configuration of:
Resource: "tenancy.x-k8s.io/v1alpha1, Resource=tenantnamespaces", GroupVersionKind: "tenancy.x-k8s.io/v1alpha1, Kind=TenantNamespace"
Name: "tenantnamespace-a", Namespace: "tenantarootns"
Object: &{map["apiVersion":"tenancy.x-k8s.io/v1alpha1" "kind":"TenantNamespace" "metadata":map["annotations":map["kubectl.kubernetes.io/last-applied-configuration":""] "labels":map["controller-tools.k8s.io":"1.0"] "name":"tenantnamespace-a" "namespace":"tenantarootns"] "spec":map["name":"tenantnamespace-a-1"]]}
from server for: "tnsa.yaml": tenantnamespaces.tenancy.x-k8s.io "tenantnamespace-a" is forbidden: User "system:serviceaccount:default:t2-adminb" cannot get resource "tenantnamespaces" in API group "tenancy.x-k8s.io" in the namespace "tenantarootns"
```

##

### Use Case 2: When `tenant-b-admin` is added in admin list of `tenant-a`

``` 
kubectl edit tenant tenant-a
```

``` 
apiVersion: tenancy.x-k8s.io/v1alpha1
kind: Tenant
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: tenant-a
spec:
  # Add fields here
  tenantAdminNamespaceName: "tenantarootns"
  tenantAdmins:
    - kind: ServiceAccount
      name: tenant-a-admin
      namespace: default
    - kind: ServiceAccount
      name: tenant-b-admin
      namespace: default
```


### Test Case 3 : 
`tenant-b-admin` will also be able to create `tenantnamespace-a` in `tenantarootns`

``` 
KUBECONFIG=/tmp/kube/k8s-tenant-a-admin-default-conf kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/test/tenantnamespace/tenantnamespace-a.yaml
KUBECONFIG=/tmp/kube/k8s-tenant-b-admin-default-conf kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/test/tenantnamespace/tenantnamespace-a.yaml
```

### Output 3:
```
tenantnamespace.tenancy.x-k8s.io/tenantnamespace-a created
```

Verify using `kubectl get tenantnamespace -n tenantarootns`










