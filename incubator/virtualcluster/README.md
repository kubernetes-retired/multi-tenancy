# Virtual Cluster

## A Short Demo

Following is a short demo by [@zhuangqh](https://github.com/zhuangqh).

[![](http://img.youtube.com/vi/QvpNehTNRyk/0.jpg)](http://www.youtube.com/watch?v=QvpNehTNRyk "vc-demo")

## How to use

1. Install tenant and tenantnamespace crd
```bash
kubectl apply -f ../../tenant/config/crds/
```
<br />
<br />

2. Start the tenant controller
```bash
kubectl apply -f ../../tenant/config/manager/all_in_one.yaml
```
<br />
<br />

3. Create a tenant CR
```bash
kubectl apply -f ../../tenant/config/samples/tenancy_v1alpha1_tenant.yaml
```
a tenant admin namespace `tenant1admin` will be created.
<br />
<br />

4. Build vcctl
```bash
# on osx
make vcctl-osx
# on linux 
make all WHAT=cmd/vcctl
```
<br />
<br />

5. Install Virtualcluster and ClusterVersion crd
```bash
kubectl apply -f config/crds
```
<br />
<br />

6. Create an independent namespace for running management controllers (i.e. 
vc-manager, vn-agent and syncer).
```bash
kubectl create ns vc-manager
```
<br />
<br />

7. when communicating with kubelets, vn-agents will act like the supert master, thus
we need to pass the kubelet client ca of the supert master to vn-agents. 
We achieve this by serializing the ca into a secret that will be mounted on 
vn-agents.
```bash
# if using minikube, the client CA (i.e. client.crt and client.key) is located in ~/.minikube/
cp ~/.minikube/client.crt ~/.minikube/client.key .
# create secret
kubectl create secret generic vc-kubelet-client --from-file=./client.crt --from-file=./client.key --namespace vc-manager
```
<br />
<br />

8. Setup the three management controllers
```bash
kubectl apply -f config/setup/all_in_one.yaml
```
<br />
<br />

9. Create the clusterversion (e.g. cv-sample), once the management 
controllers are ready.
```bash 
_output/bin/vcctl create -yaml config/sampleswithspec/clusterversion_v1.yaml
```
<br />
<br />

10. If using minikube, create the tenant namespace and virtualcluster
```bash
_output/bin/vcctl create -yaml config/sampleswithspec/virtualcluster_1.yaml -vckbcfg vc-1.kubeconfig -minikube
```
Once the tenant master is created, a kubeconfig file `vc-1.kubeconfig` will 
be created
<br />
<br />

11. Check if tenant master is up and running
```bash
$ kubectl cluster-info --kubeconfig vc-1.kubeconfig
Kubernetes master is running at https://XXX.XXX.XX.XXX:XXXXX

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```
<br />
<br />

12. There is no node registered with the Virtualcluster
```bash
$ kubectl get node --kubeconfig vc-1.kubeconfig
No resources found in default namespace.
```
<br />
<br />

13. Let's create a test deployment
```bash
kubectl apply --kubeconfig vc-1.kubeconfig -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deploy
  labels:
    app: vc-test
spec:
  replicas: 1
  selector:
    matchLabels:
      app: vc-test
  template:
    metadata:
      labels:
        app: vc-test
    spec:
      containers:
      - name: poc
        image: busybox
        command:
        - top
EOF
```
up to successful creation, we will see the newly created pod in 
views of both Virtualcluster and Metacluster 
```bash
$ kubectl get po --kubeconfig vc-1.kubeconfig
NAME                          READY   STATUS    RESTARTS   AGE
test-deploy-f5dbf6b69-vcwf6   1/1     Running   0          10s
$ kubectl get po -A
NAMESPACE             NAME                               READY   STATUS    RESTARTS   AGE
...
vc-sample-1-default   test-deploy-f5dbf6b69-vcwf6        1/1     Running   0          35s
```
also, if the pod is run on a node that was previously unkonwn 
to the Virtualcluster, the node will be registered with the Virtualcluster
```bash
$ kubectl get node --kubeconfig vc-1.kubeconfig 
NAME       STATUS   ROLES    AGE     VERSION
minikube   Ready    <none>   2m14s
```
