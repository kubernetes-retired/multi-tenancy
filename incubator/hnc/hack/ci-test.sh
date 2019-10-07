#!/usr/bin/env bash

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

set -e

echo "Installing 'kubebuilder'"
wget https://github.com/kubernetes-sigs/kubebuilder/releases/download/v2.0.0-alpha.1/kubebuilder_2.0.0-alpha.1_linux_amd64.tar.gz
tar -zxvf kubebuilder_2.0.0-alpha.1_linux_amd64.tar.gz
mv kubebuilder_2.0.0-alpha.1_linux_amd64 /usr/local/kubebuilder

echo "Installing 'kustomize'"
GO111MODULE=on go get sigs.k8s.io/kustomize/kustomize/v3@v3.2.1

hack_dir=$(dirname ${BASH_SOURCE})

echo "Running 'make kubectl'"
make kubectl -C "$hack_dir/.."

echo "Running 'make test-kustomize'"
make test-kustomize -C "$hack_dir/.."

echo "Running 'make tests'"
make test -C "$hack_dir/.."
