#!/usr/bin/env bash

# Copyright 2020 The Kubernetes Authors.
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

set -o errexit
set -o nounset
set -o pipefail

export GO111MODULE=on

VC_EXPERIMENT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
VC_EXPERIMENT_OUTPUT_DIR=${VC_EXPERIMENT_ROOT}/_output/
VC_EXPERIMENT_BIN_DIR=${VC_EXPERIMENT_OUTPUT_DIR}/bin/
VC_EXPERIMENT_RELEASE_DIR=${VC_EXPERIMENT_OUTPUT_DIR}/release/

readonly VC_DOCKER_REGISTRY="${VC_DOCKER_REGISTRY:-virtualcluster}"
readonly VC_BASE_IMAGE_REGISTRY="${VC_BASE_IMAGE_REGISTRY:-k8s.gcr.io}"

DOCKER="docker"

source "${VC_EXPERIMENT_ROOT}/hack/lib/build.sh"
source "${VC_EXPERIMENT_ROOT}/hack/lib/docker-image.sh"
source "${VC_EXPERIMENT_ROOT}/hack/lib/util.sh"
