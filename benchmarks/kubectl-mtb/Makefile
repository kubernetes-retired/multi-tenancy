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
	go build -o bin/kubectl/kubectl-mtb ./cmd/kubectl/main.go

# Clean all binaries
clean:
	rm -f ${GOPATH}/bin/kubectl-mtb

# Install kubectl plugin
kubectl: build
	cp bin/kubectl/kubectl-mtb ${GOPATH}/bin/kubectl-mtb