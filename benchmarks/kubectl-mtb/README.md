**[Installation](#setup-instructions)** |
**[How to use](#how-to-use)** |
**[How to include bechmarks](#how-to-include-benchmarks)**|

# kubectl-mtb
> kubectl plugin to validate the the multi-tenancy in the K8s cluster.
> This tool automates behavioral and configuration checks on existing clusters which will help K8s users validate whether their
clusters are set up correctly for multi-tenancy.

## Demo
<a href="https://asciinema.org/a/5J0bA6AIIk8Y0mH8w3UYSRkxK?autoplay=1&preload=1"><img src="https://asciinema.org/a/5J0bA6AIIk8Y0mH8w3UYSRkxK.svg" width="836"/></a>

## Setup Instructions

**Prerequisites** : Make sure you have working GO environment in your system.

kubectl-mtb can be installed by running

```bash
$ go get sigs.k8s.io/multi-tenancy/benchmarks/kubectl-mtb
```
or by cloning this repository, and running

```bash 
$ make kubectl-mtb
```
<hr>

## How to use

## Table of Contents
=================
* [List available benchmarks](#list-available-benchmarks)
* [Run the available benchmarks](#run-the-available-benchmarks)
* [List Policy Reports](#list-policy-reports)
* [Generate README](#generate-readme)
* [Run unittests](#run-unittests)

### List available benchmarks :

```bash
$ kubectl-mtb get benchmarks
```

### Run the available benchmarks:

```bash
$ kubectl-mtb test benchmarks -n "name of tenant-admin namespace" -t "name of tenant service account"
```
You can mention the profile level of the  benchmark using `-p` flag. 

Example: 

```bash
$ kubectl-mtb test benchmarks -n tenant0admin -t system:serviceaccount:tenant0admin:t0-admin0
```

### List Policy Reports:

```bash
$ kubectl-mtb test benchmarks -n "name of tenant-admin namespace" -t "name of tenant service account" -o policyreport
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
 
## How to include bechmarks

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