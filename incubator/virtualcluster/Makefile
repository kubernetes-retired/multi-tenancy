# Explicitly opt into go modules, even though we're inside a GOPATH directory
export GO111MODULE=on

# Image URL to use all building/pushing image targets
IMG ?= virtualcluster/manager-amd64 virtualcluster/vn-agent-amd64 virtualcluster/syncer-amd64

# TEST_FLAGS used as flags of go test.
TEST_FLAGS ?= -v --race

# COVERAGE_PACKAGES is the coverage we care about.
COVERAGE_PACKAGES=$(shell go list ./... | \
				  grep -v github.com/multi-tenancy/incubator/virtualcluster/pkg/client | \
				  grep -v github.com/multi-tenancy/incubator/virtualcluster/pkg/apis | \
				  grep -v github.com/multi-tenancy/incubator/virtualcluster/cmd)

# CRD_OPTIONS ?= "crd:trivialVersions=true"
CRD_OPTIONS ?= "crd:trivialVersions=true,maxDescLen=0"

# Build code.
#
# Args:
#   WHAT: Directory names to build.  If any of these directories has a 'main'
#     package, the build will produce executable files under $(OUT_DIR).
#     If not specified, "everything" will be built.
#   GOFLAGS: Extra flags to pass to 'go' when building.
#   GOLDFLAGS: Extra linking flags passed to 'go' when building.
#   GOGCFLAGS: Additional go compile flags passed to 'go' when building.
#
# Example:
#   make
#   make all
#   make all WHAT=cmd/kubelet GOFLAGS=-v
#   make all GOLDFLAGS=""
#     Note: Specify GOLDFLAGS as an empty string for building unstripped binaries, which allows
#           you to use code debugging tools like delve. When GOLDFLAGS is unspecified, it defaults
#           to "-s -w" which strips debug information. Other flags that can be used for GOLDFLAGS
#           are documented at https://golang.org/cmd/link/
.PHONY: all
all: test build

build: 
	hack/make-rules/build.sh $(WHAT)

# Run tests
test: generate fmt vet manifests
	@mkdir -p coverage
	@( for pkg in ${COVERAGE_PACKAGES}; do \
		go test ${TEST_FLAGS} \
			-coverprofile=coverage/unit-test-`echo $$pkg | tr "/" "_"`.out \
			$$pkg ;\
	done )

.PHONY: coverage
coverage: ## combine coverage after test
	@mkdir -p coverage
	@gocovmerge coverage/* > coverage/coverage.txt
	@go tool cover -html=coverage/coverage.txt -o coverage/coverage.html

.PHONY: clean
clean: ## clean to remove bin/* and files created by module
	@go mod tidy
	@rm -rf _output/*
	@rm -rf coverage/*

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./cmd/manager/main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crds

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crds
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role paths="./..." output:crd:artifacts:config=config/crds
	hack/make-rules/replace-null.sh

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Generate code
generate: controller-gen
ifndef GOPATH
	$(error GOPATH not defined, please define GOPATH. Run "go help gopath" to learn more about GOPATH)
endif
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build release image.
#
# 1. run tests
# 2. build docker image
.PHONY: release-images
release-images: test build-images

# Build docker image.
#
# 1. build all binaries.
# 2. copy binaries to the corresponding docker image.
build-images:
	hack/make-rules/release-images.sh $(WHAT)

# Push the docker image
docker-push:
	$(foreach i,$(IMG),docker push $i;)

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# build vcctl on osx
vcctl-osx:
	GOOS=darwin hack/make-rules/build.sh cmd/vcctl 

# Build and run kubernetes e2e tests.
#
# Args:
#  KUBECONFIG: kubeconfig to virtual cluster. If empty, create a virtual cluster.
#    Defaults to "".
#  FOCUS: Regexp that matches the tests to be run.  Defaults to "\[Conformance\]".
#  SKIP: Regexp that matches the tests that needs to be skipped.
#    Defaults to "\[Flaky\]|\[Slow\]|\[Serial\]"
#  BUILD_DEPENDENCIES: if true, build dependencies related to e2e test.
#    Defaults to true.
#
# Example:
#   make test-e2e-k8s KUBECONFIG=/path/to/vc-kubeconfig
.PHONY: test-e2e-k8s
test-e2e-k8s:
	hack/make-rules/test-e2e-k8s.sh
