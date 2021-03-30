# Supporting Multiple Super Clusters

This demo illustrates how to setup a VirtualCluster environment to support scheduling tenant Pods to multiple super clusters.

## Architecture

The demo will realize the following architecture.

<div align="left">
  <img src="./demo-arch.png" width=70% title="Architecture">
</div>

A few notes:
- Most of the CRDs, components such as the namespace scheduler, the per super cluster syncer controller, are installed in the meta cluster. 
`vn-agent` needs to be installed in each super cluster using DaemonSet (skipped in this demo).
- In this demo, the tenant cluster is created by vc-manager using the VirutalCluster CRD. The super clusters are created using existing tools, e.g., `minikube`. 
However, we leverage the CAPI [`Cluster`](https://github.com/kubernetes-sigs/cluster-api/tree/master/api/v1alpha4/cluster_types.go)
CRD to represent the super clusters so that the namespace scheduler can find the super cluster access credential.


## Environment
### Step 1: Setup the Meta Cluster

Choose an existing cluster or create a new cluster to serve as a meta cluster. Take `minikube` for an example,

```bash
minikube start -p meta
```

Besides the VirtualCluster and ClusterVersion CRDs, the Cluster CRD needs be installed as well:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy.x-k8s.io_clusterversions.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy.x-k8s.io_virtualclusters.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/cluster.x-k8s.io_clusters.yaml
```

Install vc-manager and vc-scheduler in the vc-manager namespacing using the following command:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/experiment/config/setup/all_in_one.yaml
```

Although the per super cluster syncer will be installed in the meta cluster, its configuration relies on the super cluster deployment. We will describe
its installation in Step 3.



### Step 2: Create a VirtualCluster

Follow the exact same steps in the VirtualCluster [demo](https://github.com/kubernetes-sigs/multi-tenancy/blob/master/incubator/virtualcluster/doc/demo.md#create-clusterversion) to create a VirtualCluster. For example, using the following command to create a VirtualCluster named `vc-sample-1`.

```bash
$ kubectl vc create -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/virtualcluster_1_nodeport.yaml -o vc-1.kubeconfig
```

Once it is created, a kubeconfig file, namely `vc-1.kubeconfig`, will be created in the current directory.

### Step 3: Setup the Super Clusters

We can use existing clusters as super clusters or create new super clusters using existing tools. Take `minikube` for an example:

```bash
minikube start -p $SUPER_ID
```

We have prepared a scripts (`setup-supercluster-minikube.sh`) to help set up the super clusters. This script assumes that
`minikube` was used to create the super cluster. The script takes the `$SUPER_ID` as the input
and will generate five yamls and one kubeconfig for the super cluster.

```bash
$ $VC_REPO/experiment/config/setup/setup-supercluster-minikube.sh $SUPER_ID
$ ls cluster_$SUPER_ID
cluster-cr.yaml  cluster-id.yaml  kubeconfig  secret-for-scheduler.yaml  secret-for-vc-syncer.yaml  vc-syncer.yaml
```

The reason why we create two secrets for the same super cluster is because in this demo, the scheduler requires the `Cluster` CR and the secret
to be in the same namespace (`default`) and the syncer requires the secret to exist in its own namespace (`vc-manager`). 

Next, we can apply the yamls in corresponding clusters. The `supercluster-info` configmap needs to be installed in the super cluster.
```bash
kubectl --kubeconfig kubeconfig apply -f cluster-id.yaml
```

The `Cluster` CR, vc-syncer and two secrets need to be installed in the meta cluster.

```bash
$ kubectl --context meta apply -f cluster-cr.yaml
$ kubectl --context meta apply -f secret-for-scheduler.yaml
$ kubectl --context meta apply -f vc-syncer.yaml
$ kubectl --context meta apply -f secret-for-vc-syncer.yaml
```

We can check the status of vc-syncer for the super cluster `$SUPER_ID`:

```bash
$ kubectl --context meta get deploy -n vc-manager vc-syncer-$SUPER_ID
```

We repeat this step multiple times to configure more super clusters.

## Experiment 

Assuming we have created two super clusters: r1 and r2 and one virtual cluster following the above steps,  
we use the `default` namespace in the virtual cluster to do the experiment.

First, we configure the namespace slice, which is the scheduling unit used in the namespace scheduler, by
adding an annotation `scheduler.tenancy.x-k8s.io/slice: '{"cpu":"100m", "memory":"100Mi"}'` to the `default` namespace.

Then we create a resource quota in the `default` namespace, which has the capacity of two slices in total.

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

The namespace scheduler should update the scheduling result in the `default` namespace's annotation shortly
using the key `scheduler.tenancy.x-k8s.io/placements`.

```bash
$ kubectl --kubeconfig vc-1.kubeconfig get ns default -o yaml
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

The placement result indicates that the quota is distributed to two super clusters, each has the quota of one slice.

Now we create a Deployment in the virtual cluster whose Pod has the resource request that exactly fits one slice.

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

The namespace scheduler should update the cluster placement to this Pod's annotation, saying this Pod was scheduled to the super cluster `r1`.

```bash
$ kubectl get pod -n default test-1-684cc8d565-4qnh6 -o yaml
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
```

We can increase the replica number of the Deployment to 2 , and the second Pod should be scheduled to another super cluster.

```bash
$ kubectl --context r2 get pod -A
NAMESPACE                            NAME                           READY   STATUS    RESTARTS   AGE
default-e8818d-vc-sample-1-default   test-1-74d68bc5bd-2jzfz        1/1     Running   0          19s

$ kubectl  --context r1 get pod -A
NAMESPACE                            NAME                             READY   STATUS    RESTARTS   AGE
default-e8818d-vc-sample-1-default   test-1-684cc8d565-4qnh6          1/1     Running   0          12s
```
