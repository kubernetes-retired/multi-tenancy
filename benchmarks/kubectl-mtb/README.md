**[Setup](#setup-instructions)** |
**[Usage](#usage)** |
**[Contributing](#contributing)**|

# kubectl-mtb
> kubectl plugin to validate the the multi-tenancy in the K8s cluster.
> This tool automates behavioral and configuration checks on existing clusters which will help K8s users validate whether their
clusters are set up correctly for multi-tenancy.

## Demo
[![asciicast](https://asciinema.org/a/YHqiNLWhpy596myFUCdghetwL.svg)](https://asciinema.org/a/YHqiNLWhpy596myFUCdghetwL)

## Setup Instructions

**Prerequisites** : Make sure you have working GO environment in your system.

kubectl-mtb can be installed by cloning this repository, and running

```bash 
$ make kubectl-mtb
```
<hr>

## Usage

## Table of Contents
=================
* [List available benchmarks](#list-available-benchmarks)
* [Run the available benchmarks](#run-the-available-benchmarks)
* [Create a tenant namespace](#create-a-tenant-namespace)
* [Install Kyverno or Gatekeeper to pass benchmarks](#install-kyverno-or-gatekeeper-to-pass-benchmarks)
* [List Policy Reports](#list-policy-reports)
* [Generate README](#generate-readme)
* [Run unittests](#run-unittests)

### List available benchmarks :

```bash
$ kubectl-mtb get benchmarks
```

### Run the available benchmarks:

```bash
$ kubectl-mtb test benchmarks -n "name of tenant namespace" --as "name of user/service account"
```
You can mention the profile level of the  benchmark using `-p` flag. 

Example: 

```bash
$ kubectl-mtb test benchmarks -n tenant0admin --as system:serviceaccount:tenant0admin:t0-admin0
```

### Create a tenant namespace

Install CRD 

```
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenant.yaml
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenantnamespace.yaml
```

Install tenant controller 

```bash
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/manager/all_in_one.yaml
```
Create a tenant CR 

```bash
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/samples/tenancy_v1alpha1_tenant.yaml
```

Edit the tenant CR and add your user or service account to the tenantAdmins list

```bash
$ kubectl edit tenant tenant-sample
```

```yaml
apiVersion: tenancy.x-k8s.io/v1alpha1
kind: Tenant
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: tenant-sample
spec:
  # Add fields here
  tenantAdminNamespaceName: "tenant1admin"
  tenantAdmins:
    - kind: User
      name: your-user
      namespace: 
```
User can do self service namespace creation by creating a tenantnamespace CR in tenant1admin namespace:

Example:

```
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/samples/tenancy_v1alpha1_tenantnamespace.yaml
```
This will create t1-ns1 namespace. If you want to run the benchmarks using this namespace, Make sure that the user can access the namespace using the following command. 

```
kubectl auth can-i --list --namespace=created-namespace --as your-user
```

Then, you can run the benchmarks using the following command: 

Example:

```bash
$ kubectl-mtb test benchmarks -n t1-ns1 --as divya-k8s-access
```

### Install Kyverno or Gatekeeper to pass benchmarks

You can use policies to pass the benchmarks. We are currently maintaining [Kyverno](https://github.com/nirmata/kyverno) and [Gatekeeper](https://github.com/open-policy-agent/gatekeeper) policies in this repo which are present [here](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/benchmarks/kubectl-mtb/test/policies)

To install Kyverno, you can run the following command:

```
kubectl create -f https://github.com/nirmata/kyverno/raw/master/definitions/install.yaml
```

To install Gatekeeper, run following command:

```
 $ kubectl apply -f https://raw.githubusercontent.com/open-policy-agent/gatekeeper/master/deploy/gatekeeper.yaml
```


### List Policy Reports:

```bash
$ kubectl-mtb test benchmarks -n "name of tenant namespace" --as "name of user/service account" -o policyreport
``` 

### Generate README

README can be dynamically generated for the benchmarks from `config.yaml` (present inside each benchmark folder).
Users can add additional fields too and it will be reflected in the README after running the following command from cloned
repo. 

```bash
make readme
```

### Run unittests

- The unittests run on a separate kind cluster. To run all the unittest you can run the command `make kind-test-cluster` this will create a new cluster if it cannot be found on your machine. By default, the cluster is named `kubectl-mtb-suite`, after the tests are done, the cluster will be deleted. 

- If you want to run a particular unittest, you can checkout into the particular benchmark directory and run `go test` which will create a cluster named `kubectl-mtb` which will be deleted after the tests are completed. 


*If kind cannot be found on your system the target will try to install it using `go get`*

<hr>
 
## Contributing

You can use mtb-builder to include/write other benchmarks.

Run the following command to build mtb-builder. 

```
$ make builder
```
The generated binary will create the relevant templates, needed to write the bechmark as well as associated unittest.

**Example :**

```
$ ./mtb-builder create block multitenant resources -p 1
```
Here,  `create block multitenant resources` is name of the benchmark and `-p` flag is used here to mention the profile level. The above command will generate a directory named `create_block_multitenant_resources` under which following files would be present.

- config.yaml
- create_block_multitenant_resources_test.go
- create_block_multitenant_resources.go