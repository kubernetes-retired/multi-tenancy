# Virtual Cluster

## Setup tenant master

1. Build manager
```bash
make all WHAT=cmd/manager

```

2. Build vcctl
```bash
# on osx
make vcctl-osx
# on linux 
make all WHAT=cmd/vcctl
```

3. Remove `status` subresource from crd yaml (i.e. `config/crds/tenancy_v1alpha1_clusterversion.yaml` and `config/crds/tenancy_v1alpha1_virtualcluster.yaml`) 

4. Install crd
```bash
kubectl apply -f config/crds
```

4. Setup `vc-manager`
```bash
kubectl apply -f config/setup
```

5. Create clusterversion once `vc-manager` is ready
```bash 
_output/bin/vcctl create -yaml config/sampleswithspec/clusterversion_v1.yaml
```

6. If using minikube, create virtualcluster using following command 
```bash
_output/bin/vcctl create -yaml config/sampleswithspec/virtualcluster.yaml -minikube true
```

7. Once the tenant master is created, a kubeconfig file `vc.kubeconfig` will be created

8. Check if tenant master is up and running by command 

```bash
kubectl cluster-info --kubeconfig vc.kubeconfig
```
if all goes well, the output will look like following

```bash
Kubernetes master is running at https://192.168.99.187:30443

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

## Setup vn-agent

## Setup syncer

