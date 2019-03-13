# PoC Development Guide

The Development Guide for contributing PoC (Proof-Of-Concept) code in this repository.

## Development Environment

### Environment Variables

[direnv](https://direnv.net/) is recommended for automated environment population.
With _direnv_, `cd` into the directory of the repository, 
the environment variables gets setup automatically.
Without _direnv_, `cd` into the directory of the respository and `source .envrc` will
do the same work.

### Tools

Install pre-requisites:

- [Go](https://golang.org): `1.11.4` or above

After environment variables are populated, do one-time setup of tools using:

```
devtk setup
```

## Development Toolkit

A single entry for most of development work is `devtk` command.
To add new commandlet, create `cmd-COMMAND.sh` in `tools/lib/devkit/`.

The current commands:

#### One-time Setup

```
devkit setup
```

#### Generate Code

```
devkit codegen                   # generate code for all poc projects
devkit codegen tenant-controller # generate code for tenant-controller project only
```

#### Build Binaries

```
devkit build                               # build all binaries from all projects
devkit build tenants-ctl                   # build the specified binary
devkit build tenant-controller/tenants-ctl # build the specified binary within project explicitly specified
```
