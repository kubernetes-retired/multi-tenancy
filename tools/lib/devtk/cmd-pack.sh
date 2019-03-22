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
#    devtk pack         # pack all containers from all poc projects
#    devtk pack tenants # example, build specified container
#
#    # if tenants is ambiguous across multiple projects,
#    # specify project explicitly, e.g.
#    devtk pack tenant-controller/tenants
#
# The following environment variables affects the image name and tag:
#    IMAGE_PREFIX: the prefix appended to image, e.g. gcr.io/project/
#    IMAGE_TAG: the tag of image. Default is "dev".
# without these environment variables, the image "name:dev" is created.
#
# Additionally environment variable DOCKER_BUILD_OPTS can be used to
# add additional options to "docker build".

function pack_container() {
    local project="$1" container="$2"
    echo "pack $project/$container"
    mkdir -p $REPO_ROOT/out/$project
    cp -f $REPO_ROOT/poc/$project/data/docker/$container.Dockerfile $REPO_ROOT/out/$project/
    (
        cd $REPO_ROOT/out/$project
        docker build -t ${IMAGE_PREFIX}${container}:${IMAGE_TAG:-dev} -f $container.Dockerfile $DOCKER_BUILD_OPTS .
    )
}

function find_container_and_pack() {
    # the argument is "container" or "project/container"
    local container="$(basename $1)" project="$(dirname $1)"
    if [ -z "$project" -o "$project" == "." ]; then
        project=""
        for d in $(find poc -mindepth 1 -maxdepth 1 -type d); do
            test -f $d/data/docker/$container.Dockerfile || continue
            local p="${d##poc/}"
            test -z "$project" || fatal "ambiguous name $container: $project/$container or $p/$container"
            project="$p"
        done
        test -n "$project" || fatal "unknown name $container, please use format of PROJECT/CONTAINER"
        pack_container "$project" "$container"
    fi
}

function cmd_run() {
    cd $REPO_ROOT
    if [ $# -gt 0 ]; then
        for arg; do 
            find_container_and_pack "$arg"
        done
    else
        for fn in poc/*/data/docker/*.Dockerfile; do
            local dir="$(dirname ${fn##poc/})"
            local project="${dir%%/data/*}"
            local dockerfile="$(basename $fn)"
            local container="${dockerfile%%.Dockerfile}"
            pack_container "$project" "$container" 
        done
    fi
}
