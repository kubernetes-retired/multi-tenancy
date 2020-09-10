#!/bin/bash

set -e

# This file is run by Prow during all presuubmits

# Not included or existing by default in Prow
#  - This is part of an ongoing discussion present on the issue:
#    https://github.com/kubernetes/test-infra/issues/9469;
#  - Other projects that use Prow as the CI, e.g. kubernetes-sigs/controller-runtime,
#    https://github.com/kubernetes-sigs/controller-runtime/blob/master/hack/ci-check-everything.sh,
#    also have this custom configuration;
#  - In Prow, the GOPATH is set to /home/prow/go, whereas in
#    the Docker container is /go, which is the default one.

export PATH=$(go env GOPATH)/bin:$PATH
mkdir -p $(go env GOPATH)/bin
hack_dir=$(dirname ${BASH_SOURCE})

# install and setup kind according to https://kind.sigs.k8s.io/docs/user/quick-start/
echo "Setting up Kind"
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.8.1/kind-linux-amd64
chmod +x ./kind
mv ./kind /usr/local/bin/kind
kind create cluster --name kubectl-mtb-suite

echo "Running 'make tests'"
make tests -C "$hack_dir/.."