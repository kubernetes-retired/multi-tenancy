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

4. Install crd
```bash
kubectl apply -f config/crds
```

5. Setup `vc-manager`
```bash
kubectl apply -f config/setup
```

6. Create clusterversion once `vc-manager` is ready
```bash 
_output/bin/vcctl create -yaml config/sampleswithspec/clusterversion-v1.yaml
```

7. If using minikube, create virtualcluster using following command 
```bash
_output/bin/vcctl create -yaml config/sampleswithspec/virtualcluster-1.yaml -vckbcfg v1.kubeconfig -minikube
```

8. Once the tenant master is created, a kubeconfig file `vc1.kubeconfig` will be created

9. Check if tenant master is up and running by command 

```bash
kubectl cluster-info --kubeconfig vc.kubeconfig
```
if all goes well, the output will look like following

```bash
Kubernetes master is running at https://XXX.XXX.XX.XXX:XXXXX

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

10. There can be multiple virtualclusters running simultaneously on the meta cluster, you 
can create a second virtual cluster by using other virtualcluster yaml, for example
```bash
_output/bin/vcctl create -yaml config/sampleswithspec/virtualcluster-2.yaml -vckbcfg v2.kubeconfig -minikube
```

11. Same as before, `v2.kubeconfig` will be generated once the second virtualcluster is up and running. 

## Setup vn-agent

## Setup syncer

