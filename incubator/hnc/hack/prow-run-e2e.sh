#!/bin/bash
#
# This script is used in both the postsubmit and periodic tests (see ci-test.sh
# in this directory for the presubmits, and the README file for information
# about where this is configured in Prow).
#
# This script is designed to be run in the krte image
# (https://github.com/kubernetes/test-infra/tree/master/images/krte). That image
# includes everything needed to run Docker-in-Docker, and also cleans up after
# itself. Note that Prow seems to use containerd (and if it doesn't yet, it will
# soon) so there's no Docker daemon running on nodes unless you use this image!
#
# The precise version of krte being used is specified in two places:
#
# * The Prow configs in test-infra (see README for links).
# * The 'prow-test' target in the Makefile in this directory.
#
# The Makefile contains more information about the most recent version of krte
# we're using. Please keep it in sync with the version checked into the Prow
# config. Also, when you do that, please upgrade to the latest version of Kind
# in this file too! Look down a few lines to see/update the Kind version.

set -euf -o pipefail
cd incubator/hnc

start_time="$(date -u +%s)"
echo
echo "Starting script at $(date +%Y-%m-%d\ %H:%M:%S)"

# Install and start Kind. It seems like krte cleans this up when the test is
# done.
#
# For the 'cd' thing, see https://maelvls.dev/go111module-everywhere/. Note that
# as of Go 1.15, GO111MODULE=on *is* required.
echo
echo Installing and starting Kind...
(cd && GO111MODULE=on go get sigs.k8s.io/kind@v0.9.0)
kind create cluster

echo
echo "Building and deploying HNC"
export HNC_REGISTRY=
CONFIG=kind make deploy

echo
echo "Building kubectl-hns"
CONFIG=kind make kubectl

# The webhooks take about 30 load
echo
end_time="$(date -u +%s)"
elapsed="$(($end_time-$start_time))"
echo "Test setup took $elapsed seconds."
echo "Waiting 30s for HNC to be alive..."
sleep 10
echo "... waited 10s..."
sleep 10
echo "... waited 20s..."
sleep 10
echo "... done."

export HNC_REPAIR="${PWD}/manifests/hnc-manager.yaml"
echo
echo "Starting the tests at $(date +%Y-%m-%d\ %H:%M:%S) with HNC_REPAIR=${HNC_REPAIR}"
make test-e2e

echo
echo "Finished at $(date +%Y-%m-%d\ %H:%M:%S)"
end_time="$(date -u +%s)"
elapsed="$(($end_time-$start_time))"
echo "Script took $elapsed seconds"
