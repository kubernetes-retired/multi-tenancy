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
#    devtk codegen              # generate code for all poc projects
#    devtk codegen proj1 proj2  # generate code for specified poc projects
#    devtk codegen tenant-controller    # an example

function gen_for() {
    local project="$1"
    for types_fn in $(find $project/pkg/apis -mindepth 3 -maxdepth 3 -name types.go); do
        local apiname="$(dirname ${types_fn##$project/pkg/apis/})"
        local apiver="$(basename $apiname)"
        local apipkg="sigs.k8s.io/multi-tenancy/poc/$project/pkg/apis/$apiname"
        local clientpkg="sigs.k8s.io/multi-tenancy/poc/$project/pkg/clients/$(dirname $apiname)"
        echo "deepcopy-gen $apipkg" && {
            deepcopy-gen -h "$TOOLS_DIR/lib/header.go.txt" -i "$apipkg" -O zz_generated.deepcopy
        }
        echo "client-gen $apipkg" && {
            rm -fr "${clientpkg##sigs.k8s.io/multi-tenancy/poc/}/clientset"
            client-gen -h "$TOOLS_DIR/lib/header.go.txt" -n "$apiver" -p "$clientpkg/clientset" \
                --input-base "" --input "$apipkg"
        }
        echo "lister-gen $apipkg" && {
            rm -fr "${clientpkg##sigs.k8s.io/multi-tenancy/poc/}/listers"
            lister-gen -h "$TOOLS_DIR/lib/header.go.txt" -i "$apipkg" -p "$clientpkg/listers"
        }
        echo "informer-gen $apipkg" && {
            rm -fr "${clientpkg##sigs.k8s.io/multi-tenancy/poc/}/informers"
            informer-gen -h "$TOOLS_DIR/lib/header.go.txt" -i "$apipkg" -p "$clientpkg/informers" \
                --versioned-clientset-package "$clientpkg/clientset/$apiver" \
                --listers-package "$clientpkg/listers"
        }
    done
}

function cmd_run() {
    cd $REPO_ROOT/poc
    if [ $# -gt 0 ]; then
        for arg; do gen_for "$arg"; done
    else
        for d in *; do
            test -d "$d" || continue
            test -d "$d/pkg/apis" || continue
            gen_for "$d"
        done
    fi
}
