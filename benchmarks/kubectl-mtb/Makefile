# Adding .PHONY to hold command shortcuts.
.PHONY: release

GOBIN ?= $(shell go env GOPATH)/bin
DIRS=./pkg/... ./cmd/... ./internal/... 

###################### LOCAL ARTIFACTS #########################
# Run tests
test: build
	go test ${DIRS} -coverprofile cover.out

# Builds all binaries
build:
	go build -o bin/kubectl/kubectl-mtb ./cmd/kubectl-mtb/main.go

# Clean all binaries
clean:
	rm -f ${GOPATH}/bin/kubectl-mtb

# Install kubectl plugin
kubectl-mtb: generate readme build
	cp bin/kubectl/kubectl-mtb ${GOBIN}/kubectl-mtb

# Build the Mtb-builder
builder:
	go build -o mtb-builder ./cmd/mtb_builder/main.go

# Convert config yaml as static assets 
# Generate benchmarks import file
generate:
	@go generate ./...

.PHONY: readme
readme: 
	go run docs/main.go

# Setup a cluster to be used for testing.
HAS_KIND := $(shell command -v kind;)
KIND_PROFILE := "kubectl-mtb-suite"
kind-create:
ifndef HAS_KIND
	go get -u sigs.k8s.io/kind
endif
	@if [ -z $$(kind get clusters | grep $(KIND_PROFILE)) ]; then\
		echo "Could not find $(KIND_PROFILE) cluster. Creating...";\
		kind create cluster --name $(KIND_PROFILE) --wait 5m;\
	fi
		
kind-test:
	go test ./test/benchmarks/... -v -count=1	

kind-delete:
	kind delete cluster --name $(KIND_PROFILE)

unit-tests: kind-create kind-test kind-delete