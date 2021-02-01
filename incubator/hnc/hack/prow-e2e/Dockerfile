FROM golang:1.14.2-alpine
WORKDIR /repo

# Since go modules isn't enabled by default.
ENV GO111MODULE=on

# Build static binaries; otherwise go test complains.
ENV CGO_ENABLED=0

# Install all basic dependencies. Bash is there in case you need to debug.
# Curl, Docker, git and make are not included in the golang image.
RUN apk add --no-cache bash curl docker git make
#  bash curl docker gcc git jq make openssh-client

# Install Kind
RUN go get sigs.k8s.io/kind@v0.9.0

# Install kubectl and make sure it's available in the PATH.
RUN curl -LO https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl
RUN chmod +x ./kubectl
RUN mv ./kubectl /bin

# Add the actual script and set it as the default command for this image.
COPY run-e2e.sh run-e2e.sh
RUN chmod +x ./run-e2e.sh
CMD ["./run-e2e.sh"]
