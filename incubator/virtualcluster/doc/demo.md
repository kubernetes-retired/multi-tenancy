# VirtualCluster Walkthrough Demo

This demo illustrates how to setup a VirtualCluster in an existing lightweight environment, 
be it [`minikube`](https://minikube.sigs.k8s.io/) or [`kind`](https://kind.sigs.k8s.io/docs/) Kubernetes cluster.

It should work exactly the same if you're working on any other Kubernetes distrubitions too.

For example, to spin up a `minukube` cluster:

```bash
minikube start --driver=virtualbox --cpus=4 --memory='6g' --disk-size='10g'
```

Or a `kind` cluster:

```bash
export CLUSTER_NAME="virtual-cluster" && \
kind create cluster --name ${CLUSTER_NAME} --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
- role: worker
- role: worker
EOF
```

## Build and install `kubectl-vc`

VirtualCluster offers a handy `kubectl` plugin, we can build and use it by following this process.

```bash
# Clone the repo && cd to virtualcluster folder
git clone https://github.com/kubernetes-sigs/multi-tenancy.git
cd multi-tenancy/incubator/virtualcluster

# Build it
make build WHAT=cmd/kubectl-vc
# Or build on specific OS like macOS
make build WHAT=cmd/kubectl-vc GOOS=darwin

# Install it by simply copying it over to $PATH
cp -f _output/bin/kubectl-vc /usr/local/bin
```

And then you can manage VirtualCluster by `kubectl vc` command tool.


## Install VirtualCluster CRDs and components

To install VirtualCluster CRDs:

```bash
# There is known controller runtime code gen problem so the generated CRD for clusterversions doesn't work for now
# So temporarily we use a simplified one.
# Slack conversation: https://kubernetes.slack.com/archives/C8E6YA9S7/p1604903060089400
#kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy.x-k8s.io_clusterversions.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/tenancy.x-k8s.io_clusterversions.yaml
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy.x-k8s.io_virtualclusters.yaml
```

To create all VirtualCluster components:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/setup/all_in_one.yaml
```

Let's check out what we've installed:

```bash
# a dedicated namespace named "vc-manager" is created
$ kubectl get ns
NAME              STATUS   AGE
default           Active   14m
kube-node-lease   Active   14m
kube-public       Active   14m
kube-system       Active   14m
vc-manager        Active   74s

# and the components, including vc-manager, vc-syncer and vn-agent are installed within namespace `vc-manager`
$ kubectl get all -n vc-manager
NAME                              READY   STATUS    RESTARTS   AGE
pod/vc-manager-76c5878465-mv4nv   1/1     Running   0          92s
pod/vc-syncer-55c5bc5898-v4hv5    1/1     Running   0          92s
pod/vn-agent-d9dp2                1/1     Running   0          92s

NAME                                     TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)    AGE
service/virtualcluster-webhook-service   ClusterIP   10.106.26.51   <none>        9443/TCP   76s

NAME                      DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
daemonset.apps/vn-agent   1         1         1       1            1           <none>          92s

NAME                         READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/vc-manager   1/1     1            1           92s
deployment.apps/vc-syncer    1/1     1            1           92s

NAME                                    DESIRED   CURRENT   READY   AGE
replicaset.apps/vc-manager-76c5878465   1         1         1       92s
replicaset.apps/vc-syncer-55c5bc5898    1         1         1       92s
```

## (Optional) Create `kubelet` client secrete and update `vn-agent`

By default, `vn-agent` works in a suboptimal mode by forwarding all `kubelet` API requests to super master.
A more efficient method is to communicate with `kubelet` directly using the client cert/key used by the super master.

The location of the client PKI files may vary based on the local setup.
Please note that we need to make sure the client cert/key files are imported as `client.crt` and `client.key` so that they can be referenced to.

### Create `kubelet` client secrete in `minikube` cluster

If you're using `minikube`, the client PKI files are located in `~/.minikube/`.

So we can create `vc-kubelet-client` secert using the following cmd:

```bash
# Copy the files over
cp ~/.minikube/cert.pem client.crt && cp ~/.minikube/key.pem client.key
# Create a new secret
kubectl create secret generic vc-kubelet-client --from-file=client.crt --from-file=client.key --namespace vc-manager
```

### Create `kubelet` client secrete in `kind` cluster:

If you're using `kind`, the client PKI files are located in its control plane Docker container, so we can retrieve it back and create `vc-kubelet-client` secert using the following cmd:

```bash
# Retrieve the kubelet client key/cert files
docker cp ${CLUSTER_NAME}-control-plane:/etc/kubernetes/pki/apiserver-kubelet-client.crt client.crt
docker cp ${CLUSTER_NAME}-control-plane:/etc/kubernetes/pki/apiserver-kubelet-client.key client.key
# Create a new secret
kubectl create secret generic vc-kubelet-client --from-file=client.crt --from-file=client.key --namespace vc-manager
```

### Update `vn-agent`

To apply this secret to `vn-agent` Pod, one can patch the `vn-agent` DaemonSet to change the secret name of the `kubelet-client-cert` volume to newly created `vc-kubelet-client`:

```bash
$ kubectl -n vc-manager patch daemonset/vn-agent --type json \
    -p='[{"op": "replace", "path": "/spec/template/spec/volumes/0/secret/secretName", "value":"vc-kubelet-client"}]'
```

The `vn-agent` Pod will be recreated in every node and `vn-agent` can directly talk with `kubelet` from now onwards.


## Create ClusterVersion

A `ClusterVersion` CR specifies how the tenant master(s) will be configured, as a template for tenant masters' components.

The following cmd will create a `ClusterVersion` named `cv-sample-np`, which specifies the tenant master components as:
- `etcd`: a StatefulSet with `virtualcluster/etcd-v3.4.0` image, 1 replica;
- `apiServer`: a StatefulSet with `virtualcluster/apiserver-v1.16.2` image, 1 replica;
- `controllerManager`: a StatefulSet with `virtualcluster/controller-manager-v1.16.2` image, 1 replica.

```bash
$ kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/clusterversion_v1_nodeport.yaml
```

> Note that tenant master does not have scheduler installed. The Pods are still scheduled as usual in super master.

## Create VirtualCluster

We can now create a `VirtualCluster` CR, which refers to the `ClusterVersion` that we just created.

The `vc-manager` will create a tenant master, where its tenant apiserver is exposed through nodeport service.

```bash
$ kubectl vc create -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/virtualcluster_1_nodeport.yaml -o vc-1.kubeconfig
2020/11/15 11:13:26 etcd is ready
2020/11/15 11:13:46 apiserver is ready
2020/11/15 11:14:12 controller-manager is ready
2020/11/15 11:14:12 VirtualCluster default/vc-sample-1 setup successfully
```

The command will create a tenant master named `vc-sample-1`.

Once it's created, a kubeconfig file specified by `-o`, namely `vc-1.kubeconfig`, will be created in the current directory.


## Access Virtual Cluster

The generated `vc-1.kubeconfig` can be used as a normal `kubeconfig` to access the tenant virtual cluster.

Please note that if you're working on `kind` cluster, which, by default, exposes one random host port pointing to Kubernetes' default `6443`. In this case, we need to work around it and the simplest way is to deploy a "sidecar" container as the proxy to route management traffic to the service:

```bash
# Do this only when you're wroking in `kind`:

# Retrieve the tenant namespace
$ VC_NAMESPACE="$(kubectl get VirtualCluster vc-sample-1 -o json | jq -r '.status.clusterNamespace')"

# The svc node port exposed
$ VS_SVC_PORT="$(kubectl get -n ${VC_NAMESPACE} svc/apiserver-svc -o json | jq '.spec.ports[0].nodePort')"

# Remove the container if there is any
#$ docker rm -f ${CLUSTER_NAME}-kind-proxy-${VS_SVC_PORT} || true
# Create this sidecar container
$ docker run -d --restart always \
    --name ${CLUSTER_NAME}-kind-proxy-${VS_SVC_PORT} \
    --publish 127.0.0.1:${VS_SVC_PORT}:${VS_SVC_PORT} \
    --link ${CLUSTER_NAME}-control-plane:target \
    --network kind \
    alpine/socat -dd \
    tcp-listen:${VS_SVC_PORT},fork,reuseaddr tcp-connect:target:${VS_SVC_PORT}
  
# And update the vc-1.kubeconfig
$ sed -i".bak" "s|.*server:.*|    server: https://127.0.0.1:${VS_SVC_PORT}|" vc-1.kubeconfig
```

Now let's take a look how Virtual Cluster looks like:

```bash
# A dedicated API Server, of course the <IP>:<PORT> may vary
$ kubectl cluster-info --kubeconfig vc-1.kubeconfig
Kubernetes master is running at https://192.168.99.106:31501  # in minikube cluster
Kubernetes master is running at https://127.0.0.1:30998       # or in kind cluster

# Looks exactly like a vanilla Kubernetes
$ kubectl get namespace --kubeconfig vc-1.kubeconfig
NAME              STATUS   AGE
default           Active   9m11s
kube-node-lease   Active   9m13s
kube-public       Active   9m13s
kube-system       Active   9m13s
```

But from the super master angle, we can see something different:

```bash
$ kubectl get namespace
NAME                                         STATUS   AGE
default                                      Active   30m
default-532c0e-vc-sample-1                   Active   10m
default-532c0e-vc-sample-1-default           Active   8m53s
default-532c0e-vc-sample-1-kube-node-lease   Active   8m53s
default-532c0e-vc-sample-1-kube-public       Active   8m53s
default-532c0e-vc-sample-1-kube-system       Active   8m53s
kube-node-lease                              Active   30m
kube-public                                  Active   30m
kube-system                                  Active   30m
local-path-storage                           Active   30m
vc-manager                                   Active   20m
```

## Let's do some experiments

From now on, we can view the virtual cluster as a normal cluster to work with.

```bash
# Let's create a deployment
$ kubectl apply --kubeconfig vc-1.kubeconfig -f - <<EOF
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

# Upon successful creation, there are newly created Pods in both tenant master and super master.
# a view from the tenant master
$ kubectl get pod --kubeconfig vc-1.kubeconfig
NAME                         READY   STATUS    RESTARTS   AGE
test-deploy-5f4bcd8c-9thn7   1/1     Running   0          4m44s
# a view from the super master
$ VC_NAMESPACE="$(kubectl get VirtualCluster vc-sample-1 -o json | jq -r '.status.clusterNamespace')"
$ kubectl get pod -n "${VC_NAMESPACE}-default"
NAME                         READY   STATUS    RESTARTS   AGE
test-deploy-5f4bcd8c-9thn7   1/1     Running   0          4m56

# Also, a new virtual node is created in the tenant master and tenant cannot schedule Pod on it.
$ kubectl get node --kubeconfig vc-1.kubeconfig
NAME       STATUS                     ROLES    AGE   VERSION
minikube   Ready,SchedulingDisabled   <none>   16m   v1.19.4                    # we see this in minikube cluster
virtual-cluster-worker   NotReady,SchedulingDisabled   <none>   5m8s   v1.19.1  # and this in kind cluster

# The kubelet APIs such as `logs` or `exec` should work in the tenant master.
$ VC_POD="$(kubectl get pod -l app='vc-test' --kubeconfig vc-1.kubeconfig -o jsonpath="{.items[0].metadata.name}")"

# Let's try kubectl exec
$ kubectl exec -it "${VC_POD}" --kubeconfig vc-1.kubeconfig -- /bin/sh
/ # ls
bin   dev   etc   home  proc  root  sys   tmp   usr   var

# And kubectl logs, yes we can see the logs from output of container's command "top"
$ kubectl logs "${VC_POD}" --kubeconfig vc-1.kubeconfig
Mem: 5349052K used, 739760K free, 35912K shrd, 203292K buff, 3140872K cached
CPU:  7.0% usr  5.9% sys  0.0% nic 86.5% idle  0.0% io  0.0% irq  0.3% sirq
Load average: 0.45 0.47 0.54 1/1308 23
  PID  PPID USER     STAT   VSZ %VSZ CPU %CPU COMMAND
```

## Cleanup

By deleting the VirtualCluster CR, all the tenant resources created in the super master will be deleted.

```bash
# The VirtualCluster
$ kubectl delete VirtualCluster vc-sample-1
```

Of course, you can delete all others VirtualCluster objects too to clean up everything:

```bash
# The ClusterVersion
$ kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/clusterversion_v1_nodeport.yaml

# The Virtual Cluster components
$ kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/setup/all_in_one.yaml

# The CRDs
$ kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/crds/tenancy.x-k8s.io_virtualclusters.yaml
$ kubectl delete -f https://raw.githubusercontent.com/kubernetes-sigs/multi-tenancy/master/incubator/virtualcluster/config/sampleswithspec/tenancy.x-k8s.io_clusterversions.yaml
```
