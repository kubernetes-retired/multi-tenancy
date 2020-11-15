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

# Patch singleton metadata validation to each version, e.g.
# 1) hierarchyconfiguration singleton:
#            metadata:
#              properties:
#                name:
#                  type: string
#                  enum:
#                    - hierarchy
#              type: object
# 2) hncconfig singleton:
#            metadata:
#              properties:
#                name:
#                  type: string
#                  enum:
#                    - config
#              type: object
# There are several "metadata" fields in each CRD manifest. The top-level one is
# the metadata for the CRD itself; the others are the metadata for each
# per-version schema (there may be only one if we only support one version in a
# given release). We only want to add the singleton enum validation for each
# version, so we will insert the patch after the "metadata" per-version (with
# space " " before "metadata:") and skip the top-level "metadata" (without space
# before "metadata:").
#
# NB: controller-gen puts the validation at the top-level of the CRD for
# single-version CRDs, and in a lower level for multiversion CRDs. So if you
# switch from one to the other, you may need to adjust the indenting in the two
# yaml files in this directory.
sed -i -e "/ metadata:/ r hack/crd_patches/hierarchy_singleton_enum.yaml" config/crd/bases/hnc.x-k8s.io_hierarchyconfigurations.yaml
sed -i -e "/ metadata:/ r hack/crd_patches/config_singleton_enum.yaml" config/crd/bases/hnc.x-k8s.io_hncconfigurations.yaml
