# Virtual Cluster

## Set Up Development Environment

This project uses both kubebuilder v1.0.8 and go mod. As a result, if users 
intend to change the code, they must:

- put this project inside GOAPTH and 
- set GO111MODULE=on (i.e. `export GO111MODULE=on`)

## How To Use

1. generate code 

`make generate`

2. generate crd yaml template 

`make manifests`

3. install crd to kubernetes 

`kubectl apply -f config/crds`

4. start controller 

`go run ./cmd/manager/main.go`

5. create clusterversion resources 

`go apply -f config/sampleswithspecs/clusterversion_v1.yaml`

6. create virtualcluster resources

TODO
