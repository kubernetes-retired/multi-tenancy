# Multi-Tenancy Benchmarks

This is a kubectl plugin of the multi-tenancy benchmarks, to validate the the multi-tenancy in the K8s cluster.

## Building the plugin

Makefile is used to automate the things at the local level.
**Prerequisites** : Make sure you have working GO environment in your system.

### Make commands

- **make kubectl-mtb** : To build the project and copy the binary file to the PATH.
- **make generate** : To convert benchmarks config yaml files into static assets.
- **make readme** : To generate each benchmark README files from their respective config yaml files to serve as a docs for benchmark.
