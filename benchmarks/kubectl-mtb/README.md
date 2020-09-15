# kubectl-mtb

**[Setup](#setup-instructions)** |
**[Usage](#usage)** |
**[Contributing](#contributing)**|

> kubectl plugin to validate multi-tenancy configuration for a Kubernetes cluster.

The `mtb` kubectl plugin provides behavioral and configuration checks to help validate if a cluster is properly configured for multi-tenant use. 

## Demo

[![asciicast](https://asciinema.org/a/JKaCz2rZJSb0ubDFEVOfFWBjJ.svg)](https://asciinema.org/a/JKaCz2rZJSb0ubDFEVOfFWBjJ)

## Setup

**Prerequisites** : Make sure you have working [Golang environment](https://golang.org/doc/#getting-started).

kubectl-mtb can be installed by cloning and building this repository:

```bash
git clone https://github.com/kubernetes-sigs/multi-tenancy
cd benchmarks/kubectl-mtb
make kubectl-mtb
```

The `kubectl-mtb` binary will be copied to your $GOPATH/bin directory.

## Usage

List benchmarks:

```bash
kubectl-mtb get benchmarks
```

Run benchmarks:

```bash
kubectl-mtb run benchmarks -n "namespace" --as "user"
```

## Complete example

### Create a namespace

```bash
kubectl create ns "test"
```

### Create a namespaced user role

You can use the following template to create a namespace admin role binding for a user (allie) in the namespace you want to test:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: allie
subjects:
- kind: User
  name: allie # "name" is case sensitive
roleRef:
  kind: ClusterRole
  name: admin
  apiGroup: rbac.authorization.k8s.io
```

```bash
kubectl create -n "test" -f allie.yaml
```

### Run benchmarks

```bash
kubectl-mtb run benchmarks -n "test" --as "allie"
```

Most of the benchmarks will fail, a few will pass as the user cannot access cluster resources:

<img width="577" alt="failed-tests" src="https://user-images.githubusercontent.com/21216969/89315233-3e7d6600-d698-11ea-9d3c-503521641840.png">


### Install Kyverno or Gatekeeper to secure the cluster

You can use a policy engine like [Kyverno](https://github.com/nirmata/kyverno) or [Gatekeeper](https://github.com/open-policy-agent/gatekeeper) for conformance with the benchmarks. We are currently maintaining and policies for both [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies).

#### Kyverno

To install Kyverno, you can run the following command:

```console
kubectl create -f https://github.com/nirmata/kyverno/raw/master/definitions/install.yaml
```

To apply all the Kyverno policies after installing, you can use the following command:

```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/benchmarks/kubectl-mtb/test/policies/kyverno/all_policies.yaml
```

You can learn more about Kyverno [here](https://github.com/nirmata/kyverno#quick-start).

#### Gatekeeper

To install Gatekeeper, run following command:

```console
 kubectl apply -f https://raw.githubusercontent.com/open-policy-agent/gatekeeper/master/deploy/gatekeeper.yaml
```

You can find the policies of Gatekeeper [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies/gatekeeper/)

You can refer [here](https://github.com/open-policy-agent/gatekeeper#how-to-use-gatekeeper) to know how to use Gatekeeper.

### Configure Quotas and Limits

For conformance with benchmarks like `Configure namespace resource quotas`, the namespace will also need a ResourceQuota object. To create the quota, run the following command:

```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/benchmarks/kubectl-mtb/test/quotas/ns_quota.yaml
```

After applying the policies and ResourceQuota object, run the benchmarks again. All benchmarks should pass. 

<img width="570" alt="passed-tests" src="https://user-images.githubusercontent.com/21216969/89316882-42aa8300-d69a-11ea-997a-557708fa0da0.png">


### List Policy Reports

You can output the benchmark results as a [Policy Report](https://github.com/kubernetes-sigs/wg-policy-prototypes/blob/master/policy-report/README.md)


Install the Policy Report CR:

```bash
kubectl create -f https://github.com/kubernetes-sigs/wg-policy-prototypes/raw/master/policy-report/crd/policy.kubernetes.io_policyreports.yaml
```

Run the benchmarks with the `-o policyreport` flag:

```bash
kubectl-mtb run benchmarks -n "tenantnamespace" --as "user impersonation" -o policyreport
```

## Contributing

You can use mtb-builder to add new benchmarks.

Run the following command to build mtb-builder.

```bash
make builder
```

The generated binary will create the relevant templates, needed to write the bechmark as well as associated unit test.

**Example :**

```bash
./mtb-builder create block multitenant resources -p 1
```

Here,  `create block multitenant resources` is name of the benchmark and `-p` flag is used here to mention the profile level. The above command will generate a directory named `create_block_multitenant_resources` under which following files would be present.

* config.yaml
* create_block_multitenant_resources_test.go
* create_block_multitenant_resources.go


### Generate README

A README.md can be dynamically generated for the benchmarks from `config.yaml` (present inside each benchmark folder). You can add additional fields in the `config.yaml`, and they will be reflected in the README.md after running the following command from cloned repo.

```bash
make readme
```

### Run unit tests

* The unit tests run on a separate kind cluster. To run all the unit test you can run the command `make unit-tests` this will create a new cluster if it cannot be found on your machine. By default, the cluster is named `kubectl-mtb-suite`, after the tests are done, the cluster will be deleted. 

* If you want to run a particular unit test, you can checkout into the particular benchmark directory and run `go test` which will create a cluster named `kubectl-mtb` which will be deleted after the tests are completed.

*If kind cannot be found on your system the target will try to install it using `go get`*
