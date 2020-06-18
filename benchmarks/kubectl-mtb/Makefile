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

generate:
	@go generate ./...
	@echo "[OK] Files added to embed box!"

###################### KIND ACTIONS #########################

# Creates a local kind cluster, destroying the old one if necessary.
kind:
	@echo "Warning: the 'kind' command must be in your path for this to work"
	-kind delete cluster
	kind create cluster

###################### MULTI-TENANTS ACTIONS #########################

# Creates tenants in cluster, using the https://github.com/kubernetes-sigs/multi-tenancy/blob/master/tenant/README.md#getting-started
# hard coded to create 2 tenants
# tenants: tenant-0-cr, tenant-1-cr
# tenantadmin namespaces: tenant0admin, tenant1admin
tenant:
	./hack/multitenant.sh 2