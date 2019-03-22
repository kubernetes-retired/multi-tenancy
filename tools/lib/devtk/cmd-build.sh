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
#    devtk build                # build all binaries from all poc projects
#    devtk build tenant-ctl     # example, build specified binary
#
#    # if tenant-ctl is ambiguous across multiple projects,
#    # specify project explicitly, e.g.
#    devtk build tenant-controller/tenant-ctl

function build_cmd() {
    local project="$1" cmd="$2"
    echo "build $project/$cmd"
    mkdir -p $REPO_ROOT/out/$project
    (
        cd poc/$project
        go build -o $REPO_ROOT/out/$project/$cmd ./cmd/$cmd
    )
}

function find_pkg_and_build() {
    # the argument is "cmd" or "project/cmd"
    local cmd="$(basename $1)" project="$(dirname $1)"
    if [ -z "$project" -o "$project" == "." ]; then
        project=""
        for d in $(find poc -mindepth 1 -maxdepth 1 -type d); do
            test -d $d/cmd/$cmd/main.go || continue
            local p="${d##poc/}"
            test -z "$project" || fatal "ambiguous name $cmd: $project/$cmd or $p/$cmd"
            project="$p"
        done
        test -n "$project" || fatal "unknown name $cmd, please use format of PROJECT/CMD"
        build_cmd "$project" "$cmd"
    fi
}

function cmd_run() {
    cd $REPO_ROOT
    if [ $# -gt 0 ]; then
        for arg; do 
            find_pkg_and_build "$arg"
        done
    else
        for fn in poc/*/cmd/*/main.go; do
            local dir="$(dirname ${fn##poc/})"
            local project="${dir%%/cmd/*}"
            local cmd="${dir##*/}"
            build_cmd "$project" "$cmd" 
        done
    fi
}
