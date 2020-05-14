# Build the manager binary
FROM golang:1.14 as builder

WORKDIR /workspace
# Copy the Go Modules manifests and all third-party libraries that are unlikely to change frequently
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/
COPY third_party/ third_party/

# Copy the go source
COPY api/ api/
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager ./cmd/manager/main.go

# Copied from kubebuilder scaffold to run as nonroot at
# https://github.com/kubernetes-sigs/kubebuilder/blob/7af89cb00c224c57ece37dc14ea37caf1eb769db/pkg/scaffold/v2/dockerfile.go#L60
# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER nonroot:nonroot
ENTRYPOINT ["/manager"]
