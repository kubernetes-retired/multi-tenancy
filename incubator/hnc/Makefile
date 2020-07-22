# Adding .PHONY to hold command shortcuts.
.PHONY: release

# If CONFIG is `kind`, various defaults will be optimized for deploying locally to Kind
CONFIG ?= "default"

PROJECT_ID=$(shell gcloud config get-value project)

# Set default version tag for krew build and Docker image (unless version is set)
HNC_IMG_NAME ?= hnc-manager
HNC_IMG_TAG ?= latest

# Image URL to use all building/pushing image targets
ifeq ($(CONFIG),kind)
# The tag is `kind-local` since K8s always attempst to re-pull an image with
# the `latest` tag, and this doesn't work when we're testing locally (we rely
# on the docker-push target, below, to push the image into Kind).
HNC_IMG ?= controller:kind-local
else
HNC_IMG ?= "gcr.io/${PROJECT_ID}/${HNC_IMG_NAME}:${HNC_IMG_TAG}"
# We don't want to overwrite an existing docker image and tag so we query the repository
# to see if the image already exists. This can be overridden by setting FORCE_RELEASE=true
ifeq ($(FORCE_RELEASE), true)
	IMG_READ_ERROR=1
else
	IMG_READ_ERROR=$(shell gcloud container images describe $(HNC_IMG) > /dev/null 2>&1; echo $$?)
endif
endif

HNC_RELEASED_IMG ?= "gcr.io/k8s-staging-multitenancy/${HNC_IMG_NAME}:${HNC_IMG_TAG}"

CONTROLLER_GEN ?= "./bin/controller-gen"

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
GOBIN ?= $(shell go env GOPATH)/bin

# Get check sum value of krew archive
KREW_CKSM=$(shell sha256sum bin/kubectl-hns.tar.gz | cut -d " " -f 1)

# The directories to run Go command on (excludes /vendor)
DIRS=./api/... ./cmd/... ./internal/... ./third_party/...

all: test docker-build

###################### LOCAL ARTIFACTS #########################

# Run tests
test: build
	go test ${DIRS} -coverprofile cover.out

# Builds all binaries (manager and kubectl) and manifests
build: generate fmt vet manifests
	go build -o bin/manager ./cmd/manager/main.go
	go build -o bin/kubectl/kubectl-hns ./cmd/kubectl/main.go

# Clean all binaries (manager and kubectl)
clean: krew-uninstall
	-rm -rf bin/*
	-rm -rf manifests/*
	-rm -f ${GOPATH}/bin/kubectl-hns

# Install kubectl plugin
kubectl: build
	cp bin/kubectl/kubectl-hns ${GOPATH}/bin/kubectl-hns

# Run against the configured Kubernetes cluster in ~/.kube/config
run: build
	go run ./cmd/manager/main.go --novalidation

# Install kubectl plugin locally using krew.
krew-build: krew-tar
	cp hack/krew-hierarchical-namespaces.yaml manifests/krew-hierarchical-namespaces.yaml
	sed -i 's/^\(\s*sha256\s*:\s*\).*/\1"$(KREW_CKSM)"/' \
		manifests/krew-hierarchical-namespaces.yaml
	sed -i 's/^\(\s*version\s*:\s*\).*/\1"$(HNC_IMG_TAG)"/' \
		manifests/krew-hierarchical-namespaces.yaml

# Install kubectl plugin locally using krew.
krew-install: krew-build
	kubectl krew install --manifest=manifests/krew-hierarchical-namespaces.yaml --archive=bin/kubectl-hns.tar.gz

# Make krew archive and put into /hack
krew-tar: build
	tar -zcvf bin/kubectl-hns.tar.gz bin/kubectl

# Uninstall krew
krew-uninstall:
	-kubectl krew uninstall hierarchical-namespaces

# Generate manifests e.g. CRD, RBAC etc. This can both update the generated
# files in /config (which should be checked into Git) as well as the kustomized
# files in /manifest (which are not checked into Git).
#
# Ensure that everything from ${DIRS}, above, is in "paths" in the call to
# controller-gen.  We can't say paths="./..." because that would include
# hack/tools.go, which is a fake file used to ensure that CONTROLLER_GEN gets
# vendored.
manifests: controller-gen
	@echo "Building manifests with image ${HNC_IMG}"
	@# See the comment above about the 'paths' arguments
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./api/..." paths="./cmd/..." paths="./internal/..." paths="./third_party/..." output:crd:artifacts:config=config/crd/bases
	-rm -rf manifests/
	mkdir manifests
	cd manifests && \
		touch kustomization.yaml && \
		kustomize edit add resource ../config/default && \
		kustomize edit set image controller=${HNC_IMG}
	kustomize build manifests/ -o manifests/${HNC_IMG_NAME}.yaml
	@echo "Building CRD-only manifest"
	rm manifests/kustomization.yaml
	cd manifests && \
		touch kustomization.yaml && \
		kustomize edit add resource ../config/crd
	kustomize build manifests/ -o manifests/hnc-crds.yaml

# Run go fmt against code
fmt:
	go fmt ${DIRS}

# Run go vet against code
vet:
	go vet ${DIRS}

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...

# find or download controller-gen
# download controller-gen if necessary
controller-gen: $(CONTROLLER_GEN)

$(CONTROLLER_GEN):
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
# This test will run on the current cluster the user deployed (either kind or 
# a kubernetes cluster).
.PHONY: e2e-test
e2e-test: 
	@echo "If these tests fail due to the webhook not being ready, wait 30s and try again. Note that webhooks can take up to 30s to become ready after HNC is first deployed in a cluster."
	go clean -testcache
	go test ./test/...

###################### RELEASE ACTIONS #########################
# Build the container image by Cloud Build and build YAMLs locally
#
# ALL COMMANDS THAT INCLUDE THE PERSONAL ACCESS TOKEN ARE HIDDEN
release: check-release-env test
	@echo "*********************************************"
	@echo "*********************************************"
	@echo "Invoking Cloud Build"
	@echo "*********************************************"
	@echo "*********************************************"
	@gcloud builds submit --config cloudbuild.yaml --no-source --substitutions=_HNC_IMG_TAG=${HNC_IMG_TAG},_HNC_USER=${HNC_USER},_HNC_PERSONAL_ACCESS_TOKEN=${HNC_PAT},_HNC_RELEASE_ID=${HNC_RELEASE_ID},_HNC_RELEASED_IMG=${HNC_RELEASED_IMG}
	@echo Pushing image to K8s staging
	@docker pull ${HNC_IMG}
	@docker tag ${HNC_IMG} ${HNC_RELEASED_IMG}
	@docker push ${HNC_RELEASED_IMG}

ERR_MSG=Ensure that HNC_IMG_TAG (eg v0.1.0), HNC_USER (your Github username), HNC_PAT (Github personal access token) and HNC_RELEASE_ID (Github numeric ID) are set
check-release-env:
ifndef HNC_IMG_TAG
	$(error HNC_IMG_TAG is undefined; ${ERR_MSG})
endif
ifndef HNC_USER
	$(error HNC_USER is undefinied; ${ERR_MSG})
endif
ifndef HNC_PAT
	$(error HNC_PAT is undefined; ${ERR_MSG})
endif
ifndef HNC_RELEASE_ID
	$(error HNC_RELEASE_ID is undefined; ${ERR_MSG})
endif
ifeq ($(IMG_READ_ERROR), 0)
	$(error The image ${HNC_IMG} already exists. Force and overwrite this image by using FORCE_RELEASE=true)
endif
	@echo "*********************************************"
	@echo "*********************************************"
	@echo "Releasing ${HNC_RELEASED_IMG} using ${PROJECT_ID}"
	@echo "*********************************************"
	@echo "*********************************************"
