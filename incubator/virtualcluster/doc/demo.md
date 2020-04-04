# Virtualcluster Demo

This demo illustrates how to setup a virtualcluster in an existing `minikube` Kubernetes cluster.

All virtualcluster related API resources (CRD, Secret, Configmap etc.) are created in a
tenant admin namespace. The tenant admin namespace can be created using the
[Tenant CRD](https://github.com/kubernetes-sigs/multi-tenancy/blob/master/tenant/pkg/apis/tenancy/v1alpha1/tenant_types.go),
or created manually.
If the Tenant CRD is desired, one can follow the [instructions](https://github.com/kubernetes-sigs/multi-tenancy/tree/master/tenant)
to install it. We repeat some of the steps in this demo.
 
## Create tenant admin namespace
First, we install all tenant CRDs and the tenant controller manager.
```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenant.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/crds/tenancy_v1alpha1_tenantnamespace.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/manager/all_in_one.yaml
```

Then we create a tenant CR, a tenant admin namespace `tenant1admin` will be created.
```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/tenant/config/samples/tenancy_v1alpha1_tenant.yaml
```

## Build `vcctl`
It is recommended to use `vcctl` cli tool to simplify some operations.
```bash
# on osx
make vcctl-osx
# on linux
make all WHAT=cmd/vcctl
```

The binary can be found in `_output/bin/vcctl`.

## Install CRDs and all components
Running following cmds will install all CRDs and create all virtualcluster components.
```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy_v1alpha1_clusterversion.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy_v1alpha1_virtualcluster.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/setup/all_in_one.yaml
```

For example, vc-manager, syncer Deployments and vn-agent DaemonSet can be found in namespace `vc-manager`.
```
kubectl get all -n vc-manager
NAME                             READY   STATUS    RESTARTS   AGE
pod/vc-manager-d7945f957-nh6th   1/1     Running   0          1d
pod/vc-syncer-5c6848d79f-p6wd5   1/1     Running   0          1d
pod/vn-agent-2z5zv               1/1     Running   0          1d

NAME                      DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/vn-agent   1         1         1       1            1           <none>          1d

NAME                         READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/vc-manager   1/1     1            1           1d
deployment.apps/vc-syncer    1/1     1            1           1d

NAME                                   DESIRED   CURRENT   READY   AGE
replicaset.apps/vc-manager-d7945f957   1         1         1       1d
replicaset.apps/vc-syncer-5c6848d79f   1         1         1       1d
```

## (Optional) Update client CA secret
By default, vn-agent works in a suboptimal mode by forwarding all kubelet API requests to super master.
A more efficient method is to communicate with kubelet directly using the client CA used by the super master.
The location of the client ca may vary based on the local setup.
For example, in `minikube`, the client CA files (i.e., client.crt and client.key) are located in `~/.minikube/`.
If the client CA files can be found in local setup, one can create 'vc-kubelet-client' secert using
the following cmd.
```bash
cp $PATH_TO_CA/client.crt $PATH_TO_CA/client.key .
kubectl create secret generic vc-kubelet-client --from-file=./client.crt --from-file=./client.key --namespace vc-manager
```

To use this secret in vn-agent Pod, one can edit the `vn-agent` DaemonSet and
change the secret name of the `kubelet-client-cert` volume to `vc-kubelet-client`.
The vn-agent Pod will be recreated in every node and vn-agent can directly talk with kubelet.

## Create clusterversion CR
A clusterversion CR specifies one tenant master configuration, which can be used by vc-manager to
create the tenant master components. The following cmd will create a `cv-sample-np` clusterversion CR
which specifies three StatefulSets for Kubernetes 1.15 apiserver, etcd and controller manager respectively.
```bash
_output/bin/vcctl create -yaml https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/clusterversion_v1_nodeport.yaml
```

Note that tenant master does not have scheduler installed. The Pods are scheduled in super master.

## Create virtualcluster
We can use the following cmd to create a virtualcluster CR `vc-sample-1` in `tenant1admin` namespace.
The vc-manager will create a Kubernetes 1.15 tenant master. The tenant apiserver is exposed through nodeport service
in `minikube` node.
```bash
_output/bin/vcctl create -yaml https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/virtualcluster_1_nodeport.yaml -vckbcfg vc-1.kubeconfig
```

Once the tenant master is created, a kubeconfig file `vc-1.kubeconfig` will be created in the current directory.
One can use the `vc-1.kubeconfig` to access the tenant master. For example,
```
$ kubectl cluster-info --kubeconfig vc-1.kubeconfig
Kubernetes master is running at https://XXX.XXX.XX.XXX:XXXXX
```

or
```
$ kubectl get node --kubeconfig vc-1.kubeconfig
No resources found in default namespace.
```

You can also observe that a few new namespaces are created in super master by the syncer controller.
```bash
$ kubectl get ns
NAME                                              STATUS   AGE
default                                           Active   1d
kube-node-lease                                   Active   1d
kube-public                                       Active   1d
kube-system                                       Active   1d
tenant1admin                                      Active   1d
tenant1admin-41f609-vc-sample-1                   Active   2m
tenant1admin-41f609-vc-sample-1-default           Active   2m
tenant1admin-41f609-vc-sample-1-kube-node-lease   Active   2m
tenant1admin-41f609-vc-sample-1-kube-public       Active   2m
tenant1admin-41f609-vc-sample-1-kube-system       Active   2m
vc-manager                                        Active   1d
```

## Experiments
We can create a test Deployment in the tenant master by running
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

Upon successful creation, there are newly created Pods in
both tenant master and super master.

```
$ kubectl get pod --kubeconfig vc-1.kubeconfig
NAME                          READY   STATUS    RESTARTS   AGE
test-deploy-f5dbf6b69-vcwf6   1/1     Running   0          33s

$ kubectl get pod -n tenant1admin-41f609-vc-sample-1-default
NAME                          READY   STATUS    RESTARTS   AGE
test-deploy-f5dbf6b69-vcwf6   1/1     Running   0          35s
```

Also, a new virtual node is created in the tenant master and tenant cannot schedule Pod on it.
```
$ kubectl get node --kubeconfig vc-1.kubeconfig
NAME       STATUS                     ROLES    AGE     VERSION
minikube   Ready,SchedulingDisabled   <none>   5m40s   v1.17.2
```

The kubelet APIs such as `logs` or `exec` should work in the tenant master.
```
$ kubectl exec test-deploy-f5dbf6b69-vcwf --kubeconfig vc-1.kubeconfig -it /bin/sh
/ # ls
bin   dev   etc   home  proc  root  sys   tmp   usr   var

```

## Cleanup

By deleting the virtualcluster CR, all the tenant resources created in the super master will be
deleted.


