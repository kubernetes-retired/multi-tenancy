# If CONFIG is `kind`, various defaults will be optimized for deploying locally to Kind
CONFIG ?= "default"
# Image URL to use all building/pushing image targets
ifeq ($(CONFIG),kind)
	# The tag is `kind-local` since K8s always attempst to re-pull an image with the
	# `latest` tag, and this doesn't work when we're testing locally (we rely on the
	# docker-push target, below, to push the image into Kind).
	IMG ?= controller:kind-local
else
	IMG ?= controller:latest
endif
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

.PHONY: all test manager kubectl run deploy manifests fmt vet generate docker-build docker-push controller-gen kind-reset kind-deploy kind-watch

all: manager

# Run tests
test: manager kubectl manifests
	go test ./api/... ./cmd/... ./pkg/... -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager ./cmd/manager/main.go

# Build kubectl plugin
kubectl: generate fmt vet
	go build -o bin/kubectl-hnc ./cmd/kubectl/main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: manager
	go run ./cmd/manager/main.go --novalidation

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: docker-push kubectl manifests
	kubectl apply -f manifests/hnc-manager.yaml

# Generate manifests e.g. CRD, RBAC etc. This can both update the generated
# files in /config (which should be checked into Git) as well as the kustomized
# files in /manifest (which are not checked into Git).
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	-rm -rf manifests/
	mkdir manifests
	cd manifests && \
		touch kustomization.yaml && \
		kustomize edit add resource ../config/default && \
		kustomize edit set image controller=${IMG}
	kustomize build manifests/ -o manifests/hnc-manager.yaml

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths=./api/...

# Build the docker image
docker-build: generate fmt vet
	@echo "Warning: this does not run tests. Run 'make test' to ensure tests are passing."
	docker build . -t ${IMG}

# Push the docker image
docker-push: docker-build
ifeq ($(CONFIG),kind)
	kind load docker-image ${IMG}
else
	docker push ${IMG}
endif

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.1
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

# Creates a local kind cluster, destroying the old one if necessary. It's not
# *necessary* to call this wih CONFIG=kind but it's not a bad idea either so
# the correct manifests get created.
kind-reset: manifests
	@echo "Warning: you must have sourced 'devenv' so kubectl points to the Kind cluster"
	@echo "Warning: the 'kind' command must be in your path for this to work"
	-kind delete cluster
	kind create cluster
	kubectl create namespace cert-manager
	kubectl label namespace cert-manager certmanager.k8s.io/disable-validation=true
	kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v0.8.1/cert-manager.yaml
	kubectl apply -f config/crd/bases

# Convenience target to deploy specifically for kind
kind-deploy:
	CONFIG=kind $(MAKE) deploy

kind-watch:
	kubectl logs -n hnc-system --follow deployment/hnc-controller-manager manager
