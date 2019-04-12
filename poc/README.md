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
To add new commandlet, create `cmd-COMMAND.sh` in `tools/lib/devtk/`.

Note: `devtk` is just a convinient script written from scratch for hooking up
all bash scripts that perform common development work. 

The current commands:

#### One-time Setup

```
devtk setup
```

#### Generate Code

```
devtk codegen                   # generate code for all poc projects
devtk codegen tenant-controller # generate code for tenant-controller project only
```

#### Build Binaries

```
devtk build                               # build all binaries from all projects
devtk build tenants-ctl                   # build the specified binary
devtk build tenant-controller/tenants-ctl # build the specified binary within project explicitly specified
```

#### Pack Container Image

```
devtk pack                              # pack all container images from all projects
devtk pack tenants                      # pack the specified container image
devtk pack tenant-controller/tenants    # pack the specified container image within project explicitly specified
```
