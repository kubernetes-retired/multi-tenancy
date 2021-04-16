# Adding .PHONY to hold command shortcuts.
.PHONY: release

# If CONFIG is `kind`, various defaults will be optimized for deploying locally to Kind
CONFIG ?= "default"

# The GCP project ID useful to have when performing operations that require one
# (e.g. release). If you don't have gcloud, all other operations in this
# makefile should still succeed.
#
# We use simple expansion to prevent this from being called every time we need
# the project ID. gcloud prints some noise to stderr, hence the redirect.
PROJECT_ID := ${shell gcloud config get-value project 2>/dev/null}

# The registry is the location of the image to be built, if any:
#
# * If you are building locally (e.g. 'make deploy'), this is where the image
#   will be pushed after it is built, and what the manifest files will use.
# * If you are releasing HNC, this is where the *temporary* image will be
#   built. The *final* image will be in HNC_RELEASE_REGISTRY.
#
# By default, we use the Container Registry in the current GCP project, but you
# can set it to anything, UNLESS you are calling 'make release' since the image
# needs to be built in the same project as the GCB instance that builds it.
ifdef PROJECT_ID
  HNC_REGISTRY ?= gcr.io/${PROJECT_ID}
endif

# The image name is the *base* name - excluding the registry and the tag.
HNC_IMG_NAME ?= hnc-manager

# By default, the image tag is "latest" but you should override this when
# creating a release in the form vX.Y.Z (e.g. v0.5.1).
#
# If you're using Kind, the tag is 'kind-local' since K8s always attempts to
# re-pull an image the 'latest' tag, and this doesn't work when we're testing
# locally (we rely on the docker-push target, below, to push the image into
# Kind).
ifneq ($(CONFIG),kind)
  HNC_IMG_TAG ?= latest
else
  HNC_IMG_TAG ?= kind-local
endif

# HNC_IMG is the full image name. It can't be overridden in its entirety since
# parts of the Makefile and Cloud Build target make certain assumptions about
# its components.
#
# Note that if you're using Kind, this image will never actually be pushed to
# the registry.
ifdef HNC_REGISTRY
	HNC_IMG = ${HNC_REGISTRY}/${HNC_IMG_NAME}:${HNC_IMG_TAG}
else
	HNC_IMG = ${HNC_IMG_NAME}:${HNC_IMG_TAG}
endif

# Determine the OS Name, linux/darwin
OS_NAME := $(shell go env GOOS)

# `make` must be called from the HNC root, or all kinds of things will break
# (starting with this).
CURDIR = $(shell pwd)
KUSTOMIZE ?= ${CURDIR}/hack/kustomize-3.8.1

ifeq ($(OS_NAME),darwin)
	KUSTOMIZE = $(shell which kustomize)
endif

CONTROLLER_GEN ?= ${CURDIR}/bin/controller-gen

# Enable CRD per-version validation. As of Kubernetes 1.13, we can have multiple
# versions of the Kind defined in the CRD, and use a webhook to convert between
# them. By default, KubeBuilder disables generating different validation for
# different versions of the Kind in the CRD, to be compatible with older
# Kubernetes versions.
CRD_OPTIONS ?= "crd:trivialVersions=false"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
GOBIN ?= $(shell go env GOPATH)/bin

# Get check sum value of krew archive. Note that this value is only expanded
# when the var is used.
HNC_KREW_TAR_SHA256=$(shell sha256sum bin/kubectl-hns.tar.gz | cut -d " " -f 1)

# The directories to run Go command on (excludes /vendor)
DIRS=./api/... ./cmd/... ./internal/...
GOFMT_DIRS=$(DIRS:/...=)

all: test docker-build

###################### LOCAL ARTIFACTS #########################

# Run tests
test: build
	@echo
	@echo "If tests fail due to no matches for kind \"CustomResourceDefinition\" in version \"apiextensions.k8s.io/v1\","
	@echo "please remove the old kubebuilder and reinstall it - https://book.kubebuilder.io/quick-start.html#installation"
	@echo
	go test ${DIRS} -coverprofile cover.out

# Builds all binaries (manager and kubectl) and manifests
build: generate fmt vet manifests
	go build -o bin/manager ./cmd/manager/main.go
	GOOS=linux GOARCH=amd64 go build \
	     -o bin/kubectl/kubectl-hns_linux_amd64 \
	     -ldflags="-X sigs.k8s.io/multi-tenancy/incubator/hnc/internal/version.Version=${HNC_IMG_TAG}" \
	     ./cmd/kubectl/main.go
	GOOS=darwin GOARCH=amd64 go build \
	     -o bin/kubectl/kubectl-hns_darwin_amd64 \
	     -ldflags="-X sigs.k8s.io/multi-tenancy/incubator/hnc/internal/version.Version=${HNC_IMG_TAG}" \
	     ./cmd/kubectl/main.go

# Clean all binaries (manager and kubectl)
clean: krew-uninstall
	-rm -rf bin/*
	-rm -rf manifests/*
	-rm -f ${GOPATH}/bin/kubectl-hns

# Installs the Linux kubectl plugin to $GOPATH/bin, assume that this is in your PATH already.
kubectl: build
	cp bin/kubectl/kubectl-hns_$(OS_NAME)_amd64 $(GOBIN)/kubectl-hns

# Run against the configured Kubernetes cluster in ~/.kube/config
run: build
	go run ./cmd/manager/main.go --novalidation

# Generate manifests e.g. CRD, RBAC etc. This can both update the generated
# files in /config (which should be checked into Git) as well as the kustomized
# files in /manifest (which are not checked into Git).
#
# Ensure that everything from ${DIRS}, above, is in "paths" in the call to
# controller-gen.  We can't say paths="./..." because that would include
# hack/tools.go, which is a fake file used to ensure that CONTROLLER_GEN gets
# vendored.
#
# We build the full manifest last so that the kustomization.yaml file is still
# there in case you want to rerun it manually.
manifests: controller-gen
	@echo "Building manifests with image ${HNC_IMG}"
	@# See the comment above about the 'paths' arguments
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./api/..." paths="./cmd/..." paths="./internal/..." output:crd:artifacts:config=config/crd/bases
	./hack/crd_patches/singleton-enum-patch.sh
	-rm -rf manifests/
	mkdir manifests
	@echo "Building CRD-only manifest"
	cd manifests && \
		touch kustomization.yaml && \
		${KUSTOMIZE} edit add resource ../config/crd
	${KUSTOMIZE} build manifests/ -o manifests/hnc-crds.yaml
	@echo "Building full manifest"
	rm manifests/kustomization.yaml
	cd manifests && \
		touch kustomization.yaml && \
		${KUSTOMIZE} edit add resource ../config/default && \
		${KUSTOMIZE} edit set image controller=${HNC_IMG}
	${KUSTOMIZE} build manifests/ -o manifests/${HNC_IMG_NAME}.yaml

# Run go fmt against code
fmt:
	gofmt -l -w ${GOFMT_DIRS}

# check-fmt: Checks gofmt/go fmt has been ran. gofmt -l lists files whose formatting differs from gofmt's, so it fails if there are unformatted go code.
check-fmt:
	@test -z $(shell gofmt -l ${GOFMT_DIRS})

# Run go vet against code
vet:
	go vet ${DIRS}

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...

# Use the version of controller-gen that's checked into vendor/ (see
# hack/tools.go to see how it got there).
controller-gen:
	go build -o $(CONTROLLER_GEN) sigs.k8s.io/controller-tools/cmd/controller-gen

###################### DEPLOYABLE ARTIFACTS AND ACTIONS #########################

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config.
#
# We only delete and redeploy the deployment, and nothing else, because a)
# deleting the CRDs will cause all the existing hierarchy configs to be wiped
# away and b) if we don't delete the deployment, a new image won't be pulled
# unless the tag changes.
deploy: docker-push kubectl manifests
	-kubectl -n hnc-system delete deployment hnc-controller-manager
	kubectl apply -f manifests/${HNC_IMG_NAME}.yaml

deploy-watch:
	kubectl logs -n hnc-system --follow deployment/hnc-controller-manager manager

undeploy: manifests
	@echo "********************************************************************************"
	@echo "********************************************************************************"
	@echo "********************************************************************************"
	@echo "********************************************************************************"
	@echo "This will FULLY delete HNC, including all CRDs. You have 5s to turn back"
	@echo "********************************************************************************"
	@echo "********************************************************************************"
	@echo "********************************************************************************"
	@echo "********************************************************************************"
	@sleep 5
	@echo "Deleting all CRDs to ensure all finalizers are removed"
	-kubectl delete -f manifests/hnc-crds.yaml
	@echo "Deleting the rest of HNC"
	-kubectl delete -f manifests/hnc-manager.yaml

# Push the docker image
docker-push: docker-build
	@echo "Pushing ${HNC_IMG}"
ifeq ($(CONFIG),kind)
	kind load docker-image ${HNC_IMG}
else
	docker push ${HNC_IMG}
endif

# Build the docker image
docker-build: generate fmt vet
	@echo "Warning: this does not run tests. Run 'make test' to ensure tests are passing."
	docker build . -t ${HNC_IMG}

###################### KIND ACTIONS #########################

# Creates a local kind cluster, destroying the old one if necessary.
kind-reboot:
	@echo "Warning: the 'kind' command must be in your path for this to work"
	-kind delete cluster
	kind create cluster

# Creates a local kind cluster, destroying the old one if necessary. It's not
# *necessary* to call this wih CONFIG=kind but it's not a bad idea either so
# the correct manifests get created.
kind-reset: kind-reboot
	@echo "If this didn't work, ensure you ran 'source devenv' to point kubectl at kind'"

# Convenience target to deploy specifically for kind
kind-deploy:
	CONFIG=kind $(MAKE) deploy

###################### E2E TEST #########################
# This test will run on the current cluster the user deployed (either kind or a
# kubernetes cluster).
#
# Note the `-timeout 0` that's passed to the `go test` command - by default, a
# Go test suite has a 10m timeout, and the flag disables that timeout (as of
# July 2020, these tests take ~15m and that number is expected to grow).
.PHONY: test-e2e
test-e2e: exclude-system-namespaces
	go clean -testcache
	go test -v -timeout 0 ./test/e2e/...

# This batch test will run e2e tests N times on the current cluster the user
# deployed (either kind or a kubernetes cluster), e.g. "make test-e2e-batch N=10"
.PHONY: test-e2e-batch
test-e2e-batch: exclude-system-namespaces
	number=1 ; while [[ $$number -le $N ]] ; do \
		echo $$number ; \
    ((number = number + 1)) ; \
		go clean -testcache ; \
		go test -v -timeout 0 ./test/e2e/... ; \
	done

exclude-system-namespaces:
	@echo
	@echo "Ensuring all system namespaces are excluded from HNC..."
	@kubectl label ns hnc-system hnc.x-k8s.io/excluded-namespace=true --overwrite
	@kubectl label ns kube-node-lease hnc.x-k8s.io/excluded-namespace=true --overwrite
	@kubectl label ns kube-public hnc.x-k8s.io/excluded-namespace=true --overwrite
	@kubectl label ns kube-system hnc.x-k8s.io/excluded-namespace=true --overwrite
	@echo
	@echo "If these tests fail due to the webhook not being ready, wait 30s and try again. Note that webhooks can take up to 30s to become ready"
	@echo "after HNC is first deployed in a cluster."
	@echo
ifndef HNC_REPAIR
	@echo "******************************************************"
	@echo "HNC_REPAIR IS NOT SET. CRITICAL TESTS WILL BE SKIPPED."
	@echo "You have 5s to hit Ctrl-C to cancel and try again."
	@echo "******************************************************"
	@echo
	@sleep 5
endif

###################### RELEASE ACTIONS #########################
# Build the container image by Cloud Build and build YAMLs locally
HNC_RELEASE_REGISTRY ?= gcr.io/k8s-staging-multitenancy
HNC_RELEASE_IMG = ${HNC_RELEASE_REGISTRY}/${HNC_IMG_NAME}:${HNC_IMG_TAG}
# Override this to release from a different Git repo
HNC_RELEASE_REPO_OWNER ?= kubernetes-sigs

HNC_GCB_SUBS := _HNC_REGISTRY=${HNC_RELEASE_REGISTRY}
HNC_GCB_SUBS := ${HNC_GCB_SUBS},_HNC_IMG_NAME=${HNC_IMG_NAME}
HNC_GCB_SUBS := ${HNC_GCB_SUBS},_HNC_IMG_TAG=${HNC_IMG_TAG}
HNC_GCB_SUBS := ${HNC_GCB_SUBS},_HNC_USER=${HNC_USER}
HNC_GCB_SUBS := ${HNC_GCB_SUBS},_HNC_PERSONAL_ACCESS_TOKEN=${HNC_PAT}
HNC_GCB_SUBS := ${HNC_GCB_SUBS},_HNC_RELEASE_ID=${HNC_RELEASE_ID}
HNC_GCB_SUBS := ${HNC_GCB_SUBS},_HNC_REPO_OWNER=${HNC_RELEASE_REPO_OWNER}
release: check-release-env
	@echo "*********************************************"
	@echo "*********************************************"
	@echo "Releasing ${HNC_RELEASE_IMG}"
	@echo "... override with HNC_RELEASE_REGISTRY, HNC_IMG_NAME and"
	@echo "... HNC_IMG_TAG."
	@echo "Pulling from Github multi-tenancy repo owned by ${HNC_RELEASE_REPO_OWNER}"
	@echo "... override with HNC_RELEASE_REPO_OWNER"
	@echo "GCP project: ${PROJECT_ID} (obtained from gcloud)"
	@echo "Temporary build image (must be in ${PROJECT_ID}): ${HNC_IMG}"
	@echo "Any existing images with the same tag will be overwritten!"
	@echo ""
ifeq (${HNC_FORCE_RELEASE}, true)
	@echo "HNC_FORCE_RELEASE IS ENABLED. YOU WILL PROBABLY BE OVERWRITING"
	@echo "AN EXISTING RELEASE. ARE YOU REALLY SURE ABOUT THIS????"
	@sleep 1
	@echo ""
endif
	@echo "YOU HAVE FIVE SECONDS TO CANCEL"
	@echo "*********************************************"
	@echo "*********************************************"
	@sleep 5
	@echo
	@echo "*********************************************"
	@echo "*********************************************"
	@echo "Starting build."
	@echo "*********************************************"
	@echo "*********************************************"
	gcloud builds submit --config cloudbuild.yaml --no-source --substitutions=${HNC_GCB_SUBS}
	@echo "*********************************************"
	@echo "*********************************************"
	@echo "Pushing ${HNC_IMG} to ${HNC_RELEASE_IMG}"
	@echo "*********************************************"
	@echo "*********************************************"
	@docker pull ${HNC_IMG}
	@docker tag ${HNC_IMG} ${HNC_RELEASE_IMG}
	@docker push ${HNC_RELEASE_IMG}

# Set up error checking variables
ERR_MSG=Ensure that HNC_IMG_TAG (eg v0.1.0), HNC_USER (your Github username), HNC_PAT (Github personal access token) and HNC_RELEASE_ID (Github numeric ID) are set
# During a release, We don't want to overwrite an existing docker image and tag
# so we query the repository to see if the image already exists. This can be
# overridden by setting HNC_FORCE_RELEASE=true
ifeq ($(HNC_FORCE_RELEASE), true)
	COULDNT_READ_RELEASE_IMG=1
else
	COULDNT_READ_RELEASE_IMG=$(shell gcloud container images describe $(HNC_RELEASE_IMG) > /dev/null 2>&1; echo $$?)
endif

# Actual error checking:
check-release-env:
ifndef HNC_IMG_TAG
	$(error HNC_IMG_TAG is undefined, should be vX.X.X; ${ERR_MSG})
endif
ifndef HNC_USER
	$(error HNC_USER is undefined; ${ERR_MSG})
endif
ifndef HNC_PAT
	$(error HNC_PAT is undefined; ${ERR_MSG})
endif
ifndef HNC_RELEASE_ID
	$(error HNC_RELEASE_ID is undefined; ${ERR_MSG})
endif
ifeq ($(COULDNT_READ_RELEASE_IMG), 0)
	$(error The image ${HNC_RELEASE_IMG} already exists. Force and overwrite this image by using HNC_FORCE_RELEASE=true)
endif

# Generate the Krew manifest and put it in manifests/. Note that 'manifests' must exist because
# krew-build calls krew-tar calls build calls manifests.
krew-build: krew-tar
	cp hack/krew-kubectl-hns.yaml manifests/krew-kubectl-hns.yaml
	sed -i 's/HNC_KREW_TAR_SHA256/${HNC_KREW_TAR_SHA256}/' manifests/krew-kubectl-hns.yaml
	sed -i 's/HNC_IMG_TAG/${HNC_IMG_TAG}/' manifests/krew-kubectl-hns.yaml
	sed -i 's/HNC_RELEASE_REPO_OWNER/${HNC_RELEASE_REPO_OWNER}/' manifests/krew-kubectl-hns.yaml

# This needs to be separate from krew-build so that the HNC_KREW_TAR_SHA256 env
# var can be evaluated before the recipe starts running.
krew-tar: build
	cp LICENSE bin/kubectl
	tar -zcvf bin/kubectl-hns.tar.gz bin/kubectl

# Install kubectl plugin locally using krew.
krew-install: krew-build
	kubectl krew install --manifest=manifests/krew-kubectl-hns.yaml --archive=bin/kubectl-hns.tar.gz

# Uninstall kubectl plugin locally using krew.
krew-uninstall:
	-kubectl krew uninstall hns

