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

#### You can also pass in the path to your  config. The path can either be absolute or a relative file to `benchmarks/e2e/config`
```shell 
go test ./e2e/tests -config <path-to-config>
```

### To see a more verbose output from the test

```shell
go test -v ./e2e/tests
```

### To run tests according to the Profile Levels (1, 2 and 3)

```shell
go test -v ./e2e/tests -config <path-to-config> -ginkgo.focus PL<Profile Number>
```
<br/><br/>
*Read Next >> [Contributing](contributing.md)*