# Running Validation Tests


To test your cluster, first clone the repo:

```shell
git clone https://github.com/kubernetes-sigs/multi-tenancy.git
cd multi-tenancy/benchmarks
```

## Configure Test Parameters
To set up the test configuration, edit this [config file](../config.yaml). 

### Example

````yaml
## kubeconfig of cluster admin
adminKubeconfig: <admin-kubeconfig>

tenantA:
  ## kubeconfig of tenantA
  kubeconfig: <tenant-kubeconfig>
  ## namespace of tenantA
  namespace: <tenant-namespace>
````

## Run The Tests

### Test locally

```shell
go test ./e2e/tests
```

### To see a more verbose output from the test

```shell
go test -v ./e2e/tests
```
<br/><br/>
*Read Next >> [Contributing](contributing.md)*