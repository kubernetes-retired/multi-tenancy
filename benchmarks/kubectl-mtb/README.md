# kubectl-mtb

**[Setup](#setup-instructions)** |
**[Usage](#usage)** |
**[Contributing](#contributing)**|

> kubectl plugin to validate the the multi-tenancy in the K8s cluster.
> This tool automates behavioral and configuration checks on existing clusters which will help K8s users validate whether their
clusters are set up correctly for multi-tenancy.

## Demo

[![asciicast](https://asciinema.org/a/JKaCz2rZJSb0ubDFEVOfFWBjJ.svg)](https://asciinema.org/a/JKaCz2rZJSb0ubDFEVOfFWBjJ)

## Setup Instructions

**Prerequisites** : Make sure you have working GO environment in your system.

kubectl-mtb can be installed by cloning this repository, and running

```bash
make kubectl-mtb
```

## Usage

## Table of Contents

* [List available benchmarks](#list-available-benchmarks)
* [Run the available benchmarks](#run-the-available-benchmarks)
* [Create a namespace](#create-a-namespace)
* [Create a Role and RoleBinding](#create-a-role-and-rolebinding)
* [Install Kyverno or Gatekeeper to pass benchmarks](#install-kyverno-or-gatekeeper-to-pass-benchmarks)
* [Create ResourceQuota object](#create-resourcequota-object)
* [List Policy Reports](#list-policy-reports)
* [Generate README](#generate-readme)
* [Run unit tests](#run-unit-tests)

### List available benchmarks

```bash
kubectl-mtb get benchmarks
```

### Run the available benchmarks

```bash
$ kubectl-mtb run benchmarks -n "namespace" --as "user impersonation"
```
You can mention the profile level of the  benchmark using `-p` flag. Users can switch to development mode by passing `--debug` or `-d` flag.

Example:

```bash
kubectl-mtb run benchmarks -n testnamespace --as divya-k8s-access
```

### Create a namespace

```
kubectl create ns "test"
```

### Create a namespaced user role

You can use the following template to create a role binding for a user (allie):

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

You can try running the test after creating the rolebinding, but most of the benchmarks will fail.

<img width="577" alt="failed-tests" src="https://user-images.githubusercontent.com/21216969/89315233-3e7d6600-d698-11ea-9d3c-503521641840.png">

*Some of the benchmarks passed because the user doesn't have cluster-admin privileges.*

### Install Kyverno or Gatekeeper to pass benchmarks

You can use policies to pass the benchmarks. We are currently maintaining [Kyverno](https://github.com/nirmata/kyverno) and [Gatekeeper](https://github.com/open-policy-agent/gatekeeper) policies in this repo which are present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies)

#### Kyverno

To install Kyverno, you can run the following command:

```console
kubectl create -f https://github.com/nirmata/kyverno/raw/master/definitions/install.yaml
```

To apply all the Kyverno policies after installing , you can use the below command:

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

## Create ResourceQuota object

To pass some of the benchmarks like `Configure namespace resource quotas`, you also need to create a ResourceQuota object.

To create the object, run the following command:

```console
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/benchmarks/kubectl-mtb/test/quotas/ns_quota.yaml
```

After applying the policies and ResourceQuota object, all the benchmarks should pass. 

<img width="570" alt="passed-tests" src="https://user-images.githubusercontent.com/21216969/89316882-42aa8300-d69a-11ea-997a-557708fa0da0.png">

### List Policy Reports

```bash
kubectl-mtb run benchmarks -n "tenantnamespace" --as "user impersonation" -o policyreport
```

### Generate README

README can be dynamically generated for the benchmarks from `config.yaml` (present inside each benchmark folder).
Users can add additional fields too and it will be reflected in the README after running the following command from cloned
repo.

```bash
make readme
```

### Run unit tests

* The unit tests run on a separate kind cluster. To run all the unit test you can run the command `make unit-tests` this will create a new cluster if it cannot be found on your machine. By default, the cluster is named `kubectl-mtb-suite`, after the tests are done, the cluster will be deleted. 

* If you want to run a particular unit test, you can checkout into the particular benchmark directory and run `go test` which will create a cluster named `kubectl-mtb` which will be deleted after the tests are completed.

*If kind cannot be found on your system the target will try to install it using `go get`*

## Contributing

You can use mtb-builder to include/write other benchmarks.

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
