# Explicitly opt into go modules, even though we're inside a GOPATH directory
export GO111MODULE=on

# Image URL to use all building/pushing image targets
DOCKER_REG ?= ${or ${VC_DOCKER_REGISTRY},"virtualcluster"}
IMG ?= ${DOCKER_REG}/scheduler-amd64

# TEST_FLAGS used as flags of go test.
TEST_FLAGS ?= -v --race


# CRD_OPTIONS ?= "crd:trivialVersions=true"
CRD_OPTIONS ?= "crd:trivialVersions=true,maxDescLen=0"

.PHONY: all
all: build

build: 
	hack/make-rules/build.sh $(WHAT)

.PHONY: clean
clean: ## clean to remove bin/* and files created by module
	@go mod tidy
	@rm -rf _output/*
	@rm -rf coverage/*

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...


# Build docker image.
#
# 1. build all binaries.
# 2. copy binaries to the corresponding docker image.
build-images:
	hack/make-rules/release-images.sh $(WHAT)

# Push the docker image
docker-push:
	$(foreach i,$(IMG),docker push $i;)
