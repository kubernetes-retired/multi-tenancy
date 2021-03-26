# VirtualCluster ClusterPooling Walkthrough Demo

This demo illustrates how to setup a VirtualCluster and scheduling pods to multi clusters
which are set up by [`minikube`](https://minikube.sigs.k8s.io/). 

## Create Meta Cluster

First create a meta cluster to deploy VirtualCluster controller components and simply named it with the name "meta".

```bash
minikube start -p meta
```

### Install VirtualCluster CRDs and components

To install VirtualCluster CRDs:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/cluster.x-k8s.io_clusters.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy.x-k8s.io_clusterversions.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy.x-k8s.io_virtualclusters.yaml
```

To install vc-manager and vc-scheduler:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/experiment/config/setup/all_in_one.yaml
```

Let's check out what we've installed:

```bash
# A dedicated namespace named "vc-manager" is created
$ kubectl get ns
NAME              STATUS   AGE
default           Active   14m
kube-node-lease   Active   14m
kube-public       Active   14m
kube-system       Active   14m
vc-manager        Active   74s

# And the components, including vc-manager and vc-scheduler are installed within namespace `vc-manager`
$ kubectl --context meta get all -n vc-manager
NAME                                READY   STATUS    RESTARTS   AGE
pod/vc-manager-76c5878465-l4vzd     1/1     Running   10         3d7h
pod/vc-scheduler-689c978648-mnsw8   1/1     Running   0          16m

NAME                                     TYPE        CLUSTER-IP       EXTERNAL-IP   PORT(S)    AGE
service/virtualcluster-webhook-service   ClusterIP   10.103.157.228   <none>        9443/TCP   3d7h

NAME                           READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/vc-manager     1/1     1            1           3d7h
deployment.apps/vc-scheduler   1/1     1            1           6h3m

NAME                                      DESIRED   CURRENT   READY   AGE
replicaset.apps/vc-manager-76c5878465     1         1         1       3d7h
replicaset.apps/vc-scheduler-689c978648   1         1         1       6h3m
```

## Create VirtualCluster

Create a ClusterVersion.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/clusterversion_v1_nodeport.yaml
```

Create a VirtualCluster refers to the `ClusterVersion` that we just created.

The `vc-manager` will create a tenant master, where its tenant apiserver can be exposed through nodeport, or load balancer.

```bash
$ kubectl vc create -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/virtualcluster_1_nodeport.yaml -o vc-1.kubeconfig
2021/03/24 11:13:26 etcd is ready
2021/03/24 11:13:46 apiserver is ready
2021/03/24 11:14:12 controller-manager is ready
2021/03/24 11:14:12 VirtualCluster default/vc-sample-1 setup successfully
```

The command will create a tenant master named `vc-sample-1`, exposed by NodePort.

Once it's created, a kubeconfig file specified by `-o`, namely `vc-1.kubeconfig`, will be created in the current directory.

## Create Super Cluster

There is a script help us to create a super cluster and related manifest from minikube. We will create two super cluster in this demo.
They are `r1` and `r2`. Let's try to set up one of them.

````bash
$ experiment/config/setup/create-cluster-minikube.sh r1
$ ls cluster_r1
cluster-cr.yaml           cluster-id.yaml           config-for-scheduler.yaml config-for-vc-syncer.yaml kubeconfig                vc-syncer.yaml
````

Create the identity configmap on the super cluster `r1`, it is used to identity this super cluster in vc-syncer and vc-scheduler.

```bash
kubectl --kubeconfig kubeconfig apply -f cluster-id.yaml
```

Register super cluster `r1` to vc-scheduler and deploy the standalone vc-syncer for cluster `r1` on meta cluster.

```bash
kubectl --context meta apply -f cluster-cr.yaml
kubectl --context meta apply -f config-for-scheduler.yaml
kubectl --context meta apply -f config-for-vc-syncer.yaml
kubectl --context meta apply -f vc-syncer.yaml
```

Let's check the status of vc-syncer for cluster `r1`.

```bash
# kubectl --context meta get deploy -n vc-manager vc-syncer-r1
NAME           READY   UP-TO-DATE   AVAILABLE   AGE
vc-syncer-r1   1/1     1            1           31h
```

So does cluster `r2`.

## Create Pod on VirtualCluster

First configure namespace slice for ns `default`, this is the ns scheduling unit. Add an annotation
`scheduler.tenancy.x-k8s.io/placements: '{"r1":1,"r2":1}'` to ns `default`.

Then create a quota for ns `default`, it totally contains two slices.

```bash
apiVersion: v1
kind: ResourceQuota
metadata:
  name: mem-cpu-demo
  namespace: default
spec:
  hard:
    cpu: 200m
    memory: 200Mi
```

If everything works fine, vc-scheduler will place the scheduling result on the ns annotation.

```bash
$ kubectl --kubeconfig vc-1.kubeconfig get ns default -oyaml
apiVersion: v1
kind: Namespace
metadata:
  annotations:
    scheduler.tenancy.x-k8s.io/placements: '{"r1":1,"r2":1}'
    scheduler.tenancy.x-k8s.io/slice: '{"cpu":"100m", "memory":"100Mi"}'
  creationTimestamp: "2021-03-25T02:20:20Z"
  name: default
  resourceVersion: "356"
  selfLink: /api/v1/namespaces/default
  uid: c02578ac-2539-467b-8dc4-2e0135b4c66d
spec:
  finalizers:
  - kubernetes
status:
  phase: Active
```

As we see, vc-scheduler schedule ns to different clusters separately.

It's time to create a pod on this virtualcluster. There is a pod exactly fit one slice resource.

```bash
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-1
  labels:
    app: test-1
spec:
  replicas: 1
  selector:
    matchLabels:
      app: test-1
  template:
    metadata:
      labels:
        app: test-1
    spec:
      dnsPolicy: ClusterFirst
      containers:
      - name: test
        command:
        - sh
        - -c
        - "sleep 1000000"
        image: docker.io/library/busybox:1.29
        imagePullPolicy: Always
        resources:
          limits:
            cpu: 100m
            memory: 100Mi
```

Let's see what happen to this pod. vc-scheduler update the placement to this pod, it says this pod was scheduled to cluster `r1`.
```bash
# kubectl get pod -n default     test-1-684cc8d565-4qnh6 -o yaml
apiVersion: v1
kind: Pod
metadata:
  annotations:
    scheduler.tenancy.x-k8s.io/superCluster: r1
  creationTimestamp: "2021-03-25T16:09:33Z"
  generateName: zhijin-test-2-684cc8d565-
  labels:
    app: test-1
    pod-template-hash: 684cc8d565
  name: test-1-684cc8d565-4qnh6
  namespace: default
  ownerReferences:
  - apiVersion: apps/v1
    blockOwnerDeletion: true
    controller: true
    kind: ReplicaSet
    name: test-1-684cc8d565
    uid: 33657b29-2300-46b9-ba1d-8ae96259da7a
  resourceVersion: "1416"
  selfLink: /api/v1/namespaces/default/pods/test-1-684cc8d565-4qnh6
  uid: cb2ba310-de62-42b1-9f41-b8e8c5f6201a
```

Create another pod, and it would be scheduled to another super cluster. It means VirtualCluster pods are running on different clusters separately

```bash
$ kubectl --context r2 get pod -A
NAMESPACE                            NAME                           READY   STATUS    RESTARTS   AGE
default-e8818d-vc-sample-1-default   test-1-74d68bc5bd-2jzfz        1/1     Running   0          19s
kube-system                          coredns-74ff55c5b-zf5s9        1/1     Running   5          3d8h
kube-system                          etcd-r2                        1/1     Running   5          3d8h
kube-system                          kube-apiserver-r2              1/1     Running   5          3d8h
kube-system                          kube-controller-manager-r2     1/1     Running   5          3d8h
kube-system                          kube-proxy-wcx6r               1/1     Running   5          3d8h
kube-system                          kube-scheduler-r2              1/1     Running   5          3d8h
kube-system                          storage-provisioner            1/1     Running   13         3d8h
$ kubectl  --context r1 get pod -A
NAMESPACE                            NAME                             READY   STATUS    RESTARTS   AGE
default-e8818d-vc-sample-1-default   test-2-684cc8d565-4qnh6          1/1     Running   0          12s
kube-system                          coredns-74ff55c5b-6z6mz          1/1     Running   6          3d8h
kube-system                          etcd-r1                          1/1     Running   6          3d8h
kube-system                          kube-apiserver-r1                1/1     Running   6          3d8h
kube-system                          kube-controller-manager-r1       1/1     Running   6          3d8h
kube-system                          kube-proxy-mwlnb                 1/1     Running   6          3d8h
kube-system                          kube-scheduler-r1                1/1     Running   6          3d8h
kube-system                          storage-provisioner              1/1     Running   17         3d8h
```
