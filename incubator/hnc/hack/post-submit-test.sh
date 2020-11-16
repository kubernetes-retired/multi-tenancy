# This file is run by Prow during all postsubmits
export PATH=$(go env GOPATH)/bin:$PATH
mkdir -p $(go env GOPATH)/bin

echo "Installing 'kubebuilder' to include the Ginkgo test suite requirements"
kb=2.3.1
wget https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${kb}/kubebuilder_${kb}_linux_amd64.tar.gz
tar -zxvf kubebuilder_${kb}_linux_amd64.tar.gz
mv kubebuilder_${kb}_linux_amd64 /usr/local/kubebuilder

# install and setup kind according to https://kind.sigs.k8s.io/docs/user/quick-start/
echo "Setting up Kind"
curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.8.1/kind-linux-amd64
chmod +x ./kind
mv ./kind /usr/local/bin/kind
kind create cluster

hack_dir=$(dirname ${BASH_SOURCE})

echo "Running 'make test-e2e'"
cd "$hack_dir/.."
make kind-deploy
make test-e2e
