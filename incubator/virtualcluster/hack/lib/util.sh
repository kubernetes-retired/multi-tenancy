#!/usr/bin/env bash

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

# Wait for background jobs to finish. Return with
# an error status if any of the jobs failed.
wait-for-jobs() {
  local fail=0
  local job
  for job in $(jobs -p); do
    wait "${job}" || fail=$((fail + 1))
  done
  return ${fail}
}

# Replaces the `conditions: null` and `storedVersions: null` to 
# `conditions: []` and `storedVersions: []`
# 
# NOTE: this is a hack. controller-gen@0.1.1 uses null to 
# represent empty array in yaml, which will cause `kubectl apply -f` 
# to fail. Due to dependencies issue, we will stick with this version 
# of controller-gen for now. 
# TODO replace controller-gen, and remove this hack 
replace-null() {
  for f in config/crds/*; do
    tac $f \
      | awk -F: 'NR==1, NR==2{$2="[]"}1' OFS='\: ' \
      | tac > $f.tmp && mv $f.tmp $f
  done
}
