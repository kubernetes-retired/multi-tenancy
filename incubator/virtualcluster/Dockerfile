# Build the manager binary
FROM golang:1.12 as builder

ENV GO111MODULE=on

WORKDIR /go/virtualcluster

COPY go.mod .
COPY go.sum .

RUN go mod download

COPY pkg/    pkg/
COPY cmd/    cmd/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager github.com/kubernetes-sigs/multi-tenancy/incubator/virtualcluster/cmd/manager

# Copy the controller-manager into a thin image
FROM ubuntu:latest
WORKDIR /
COPY --from=builder /go/virtualcluster/manager .
ENTRYPOINT ["/manager"]
