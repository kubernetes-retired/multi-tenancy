#!/bin/bash
# This script is designed to be run in the hnc-postsubmit-tests container. See
# https://github.com/kubernetes-sigs/multi-tenancy/blob/master/incubator/hnc/hack/prow-e2e/README.md
# for details.

set -euf -o pipefail

start_time="$(date -u +%s)"
echo
echo "Starting at $(date +%Y-%m-%d\ %H:%M:%S)"

echo
echo "Cloning repo into ${PWD}"
git clone https://github.com/kubernetes-sigs/multi-tenancy
cd multi-tenancy/incubator/hnc

# No-one else seems to clean up their Kind clusters, so I don't think we need to
# either? Does Prow handle it? To make it easier to debug locally, let's just
# give the cluster a random name (max 32k). This should match the name we look
# for in the "clean" target in the Makefile in this directory in the git repo.
CLUSTERNAME="hnc-postsubmit-${RANDOM}"
echo
echo "Creating KIND cluster '${CLUSTERNAME}' and setting kubectl context"
kind create cluster --name ${CLUSTERNAME}

echo
echo "Building HNC artifacts"
# Because we don't use the default Kind cluster name, the builtin "docker push"
# in the makefile won't work here.
CONFIG=kind make manifests
CONFIG=kind make kubectl
CONFIG=kind make docker-build

# Load image into Kind and deploy
export HNC_REPAIR="${PWD}/manifests/hnc-manager.yaml"
echo
echo "Setting HNC_REPAIR to ${HNC_REPAIR} and deploying HNC"
# Assume the default value of ${HNC_IMG} in the makefile is used
kind load docker-image --name ${CLUSTERNAME} hnc-manager:kind-local
kubectl apply -f ${HNC_REPAIR}

# The webhooks take about 30 load
echo
echo "Waiting 30s for HNC to be alive"
sleep 10
echo "... still waiting ..."
sleep 10
echo "... almost done ..."
sleep 10

echo
echo "Running the tests"
make test-e2e

# Note that this won't run if the tests fail - see above. But we may as well
# *try* to clean up after ourselves.
kind delete clusters ${CLUSTERNAME}

echo "Finished at $(date +%Y-%m-%d\ %H:%M:%S)"
end_time="$(date -u +%s)"
elapsed="$(($end_time-$start_time))"
echo "Script took $elapsed seconds"
