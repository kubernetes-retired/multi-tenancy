#  Copyright 2018 The Kubernetes Authors.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.

# Usage:
#    devtk setup

function cmd_run() {
    set -x
    # get packages required by code-generator
    go get k8s.io/apimachinery/pkg/apis/meta/v1
    go get k8s.io/api/core/v1
    go get k8s.io/api/rbac/v1
    # install code-generator
    go get k8s.io/code-generator/cmd/...
    go install k8s.io/code-generator/cmd/...
    # recreate the symbolic link
    rm -f "$GOPATH/src/sigs.k8s.io/multi-tenancy"
    mkdir -p "$GOPATH/src/sigs.k8s.io"
    ln -s "$REPO_ROOT" "$GOPATH/src/sigs.k8s.io/multi-tenancy"
}
