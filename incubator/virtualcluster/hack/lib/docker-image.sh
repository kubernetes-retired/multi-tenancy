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

# Get the set of binaries that run in Docker (on Linux)
# Entry format is "<name-of-binary>,<base-image>".
# Binaries are placed in /usr/local/bin inside the image.
#
# $1 - server architecture
get_docker_wrapped_binaries() {
  local arch=$1
  local debian_base_version=v1.0.0
  local debian_iptables_version=v11.0.2
  local targets=()
  for target in ${@:2}; do
      targets+=($target,${VC_BASE_IMAGE_REGISTRY}/debian-base-${arch}:${debian_base_version})
  done

  if [ ${#targets[@]} -eq 0 ]; then
    ### If you change any of these lists, please also update VC_ALL_TARGETS
    targets=(
      manager,"${VC_BASE_IMAGE_REGISTRY}/debian-base-${arch}:${debian_base_version}"
      syncer,"${VC_BASE_IMAGE_REGISTRY}/debian-base-${arch}:${debian_base_version}"
      vn-agent,"${VC_BASE_IMAGE_REGISTRY}/debian-base-${arch}:${debian_base_version}"
    )
  fi

  echo "${targets[@]}"
}


# This builds all the release docker images (One docker image per binary)
# Args:
#  $1 - binary_dir, the directory to save the tared images to.
#  $2 - arch, architecture for which we are building docker images.
create_docker_image() {
  local binary_dir="$1"
  local arch="$2"
  local binary_name
  local binaries=($(get_docker_wrapped_binaries "${arch}" "${@:3}"))

  for wrappable in "${binaries[@]}"; do

    local oldifs=$IFS
    IFS=","
    set $wrappable
    IFS=$oldifs

    local binary_name="$1"
    local base_image="$2"
    local docker_build_path="${binary_dir}/${binary_name}.dockerbuild"
    local docker_file_path="${docker_build_path}/Dockerfile"
    local binary_file_path="${binary_dir}/${binary_name}"
    local docker_image_tag="${VC_DOCKER_REGISTRY}/${binary_name}-${arch}:latest"

    echo "Starting docker build for image: ${binary_name}-${arch}"
    (
      rm -rf "${docker_build_path}"
      mkdir -p "${docker_build_path}"
      ln "${binary_dir}/${binary_name}" "${docker_build_path}/${binary_name}"
      cat <<EOF > "${docker_file_path}"
FROM ${base_image}
COPY ${binary_name} /usr/local/bin/${binary_name}
EOF
      "${DOCKER[@]}" build -q -t "${docker_image_tag}" "${docker_build_path}" >/dev/null
    ) &
  done

  wait-for-jobs || { echo "previous Docker build failed"; return 1; }
  echo "Docker builds done"
}

# Package up all of the binaries in docker images
build_images() {
  # Clean out any old images
  rm -rf "${VC_RELEASE_DIR}"
  mkdir -p "${VC_RELEASE_DIR}"
  cd ${VC_BIN_DIR}
  local targets=()

  for arg; do
    targets+=(${arg##*/})
  done
  echo ${targets[@]}
  
  if [ ${#targets[@]} -eq 0 ]; then
    cp "${VC_ALL_BINARIES[@]/#/}" ${VC_RELEASE_DIR}
  else
    cp ${targets[@]} ${VC_RELEASE_DIR}
  fi

  create_docker_image "${VC_RELEASE_DIR}" "amd64" "${targets[@]}"
}
