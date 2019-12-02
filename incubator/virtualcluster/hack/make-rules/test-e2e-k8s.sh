#!/usr/bin/env bash

set -ex

# Copyright 2019 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

KUBERNETES_VERSION=${KUBERNETES_VERSION:-"release-1.15"}
KUBERNETES_REPO=${KUBERNETES_REPO:-"https://github.com/kubernetes/kubernetes"}
KUBECTL_PATH=${KUBECTL_PATH:-"/usr/local/bin/kubectl"}
BUILD_DEPENDENCIES=${BUILD_DEPENDENCIES:-"true"}

# CRI_SKIP skips the test to skip.
DEFAULT_SKIP="\[Flaky\]|\[Slow\]|\[Serial\]"
export SKIP=${SKIP:-${DEFAULT_SKIP}}

# FOCUS focuses the test to run.
DEFAULT_FOCUS="\[Conformance\]"
export FOCUS=${FOCUS:-${DEFAULT_FOCUS}}

# keep the first one only
GOPATH="${GOPATH%%:*}"

KUBERNETES_PATH="${GOPATH}/src/k8s.io/kubernetes"
if [[ ! -d "${KUBERNETES_PATH}" ]]; then
  mkdir -p "${KUBERNETES_PATH}"
  cd "${KUBERNETES_PATH}"
  git clone ${KUBERNETES_REPO} .
fi
cd "${KUBERNETES_PATH}"
git pull
git checkout ${KUBERNETES_VERSION}

e2e-k8s::build_dependencies() {
  cd ${KUBERNETES_PATH}

  # install ginkgo
  make ginkgo

  # build test target
  make all WHAT=test/e2e/e2e.test
}

e2e-k8s::run() {
  export KUBERNETES_CONFORMANCE_TEST=y
  export KUBECTL_PATH=${KUBECTL_PATH}
  export KUBECONFIG=${KUBECONFIG}

  if [[ "${BUILD_DEPENDENCIES}" = true ]]; then
    e2e-k8s::build_dependencies
  fi

  if [[ "${KUBECONFIG}" = "" ]]; then
    # TODO: setup a virtual cluster
    echo "empty kubeconfig"
    return
  fi

  cd ${KUBERNETES_PATH}
  go run hack/e2e.go -- \
    --test \
    --test_args="--ginkgo.focus=${FOCUS} --ginkgo.skip=${SKIP}" \
    --provider=local
}

main() {
  e2e-k8s::run
}

main
