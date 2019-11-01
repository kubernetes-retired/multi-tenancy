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

5. Setup kubelet-client secret
```bash
kubectl create ns vc-manager
kubectl create secret generic vc-kubelet-client --from-file=./kubelet-client.crt --from-file=./kubelet-client.key --namespace vc-manager
```
put the kubelet-client cert from super master apiserver into a secret, which would be mounted into the vn-agent.

6. Setup virtualcluster control plane
```bash
kubectl apply -f config/setup/all_in_one.yaml
```
the virtualcluster controller `vc-manager` will run as a deployment, which is binded to the 
`vc-manager` service account and run in an independent namespace `vc-manager`.

 `syncer` running in a normal deployment.
 `vn-agent` running as a daemonset using hostNetwork mode.

7. Create clusterversion once `vc-manager` is ready
```bash 
_output/bin/vcctl create -yaml config/sampleswithspec/clusterversion_v1.yaml
```

8. If using minikube, create the tenant namespace and virtualcluster by using following command 
```bash
kubectl create ns tenant-1 && _output/bin/vcctl create -yaml config/sampleswithspec/virtualcluster_1.yaml -vckbcfg v1.kubeconfig -minikube
```

9. Once the tenant master is created, a kubeconfig file `v1.kubeconfig` will be created

10. Check if tenant master is up and running by command 

```bash
kubectl cluster-info --kubeconfig vc1.kubeconfig
```
if all goes well, the output will look like following

```bash
Kubernetes master is running at https://XXX.XXX.XX.XXX:XXXXX

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

11. There can be multiple virtualclusters running simultaneously on the meta cluster, you 
can create a second virtual cluster by using other virtualcluster yaml, for example
```bash
# same as before, we manully creat a tenant namespace
kubectl create ns tenant-2 && _output/bin/vcctl create -yaml config/sampleswithspec/virtualcluster_2.yaml -vckbcfg v2.kubeconfig -minikube
```
12. As previously, `v2.kubeconfig` will be generated once the second virtualcluster is up and running. 

13. You can delete existing virtualclusters by typing command
```bash
_output/bin/vcctl delete -yaml config/sampleswithspec/virtualcluster_2.yaml 
```
