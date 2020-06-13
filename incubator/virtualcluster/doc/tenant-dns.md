# Tenant DNS Support

Commonly used dns servers are not tenant-aware. Tenant Pods cannot leverage the super master
dns service because it only recognizes the super master namespaces, not the namespaces (created in tenant masters)
used by tenant Pods for DNS query. Hence, dedicated dns server has to be installed in every
tenant master that needs DNS service. Let us take `coredns` for an example in this document.

## Problem

The `coredns` installed in tenant master normally works for Pod and headless service endpoint
FQDN (Fully Qualified Domain Name) translation.
However, it cannot provide the correct cluster IP for service FQDN issued by tenant Pods. 
This is because `coredns` only watches the tenant master and stores the tenant cluster IP in its
dns records. However, tenant Pods that run in super master would expect to access the cluster IP of the
synced super master service since it is the actual cluster IP managed by the Kube-proxy in
physical nodes. The `coredns` would only work for service FQDN query had the cluster IPs of the tenant 
master service and the synced super master service been the same. Unfortunately, this is difficult
to achieve even if both the tenant master and the super master use the same service CIDR because:
- The cluster IPs have to be allocated individually in different masters in order to avoid cluster IP conflict.
- The service cluster IP is immutable once the service object is created.

As a consequence, `coredns` will always points to the tenant cluster IP for service FQDN query 
which is a bogus address for tenant Pod to access.

## Solution

The syncer might recreate the tenant master service after super master service is created using
the cluster IP allocated in the super master. This, however, would significantly complicate the syncer 
implementation and be error prone. By closely investigating all possible solutions, we found the
simplest workaround is to introduce a trivial change to the `coredns` Kubernetes plugin. The idea
is that the syncer can back populate the cluster IP used in the super master to tenant service
annotation instead of recreating a new service object and let `coredns` record the
super master cluster IP in its internal structures. The modification to `coredns` is literally one
line change:
``` bash
diff --git a/plugin/kubernetes/object/service.go b/plugin/kubernetes/object/service.go
index 295715e2..155944e8 100644
--- a/plugin/kubernetes/object/service.go
+++ b/plugin/kubernetes/object/service.go
@@ -44,7 +44,7 @@ func toService(skipCleanup bool, obj interface{}) interface{} {
                Name:         svc.GetName(),
                Namespace:    svc.GetNamespace(),
                Index:        ServiceKey(svc.GetName(), svc.GetNamespace()),
-               ClusterIP:    svc.Spec.ClusterIP,
+               ClusterIP:    svc.Annotations["transparency.tenancy.x-k8s.io/clusterIP"],
                Type:         svc.Spec.Type,
                ExternalName: svc.Spec.ExternalName,

```

Let us describe the steps to create a customized `coredns` container image with above change.
They are simple.
1. Get the `coredns` source code with desired version (e.g., v1.6.8).
```
git clone https://github.com/coredns/coredns.git
git checkout tags/v1.6.8 
```

2. In the source code root directory, use the following cmd to change the code. Note that
the syncer has populated the super master cluster IP in the tenant master service annotation using
the key `transparency.tenancy.x-k8s.io/clusterIP`.
```
sed -i'' -e 's/svc.Spec.ClusterIP/svc.Annotations["transparency.tenancy.x-k8s.io\/clusterIP"]/g' plugin/kubernetes/object/service.go
```

3. Compile and build the new image.
```
make -f Makefile.release DOCKER=virtualcluster LINUX_ARCH=amd64 release
make -f Makefile.release DOCKER=virtualcluster LINUX_ARCH=amd64 docker
```

Now the customized `coredns` image is ready. We have also prepared a few images for different `coredns`
versions in the virtualcluster docker hub repo.

## Installation

Assuming is a virtualcluster has been installed (e.g., using the instructions in the [demo](demo.md)),
one can use the provided sample coredns [yaml](../config/sampleswithspec/coredns.yaml) to install the customized
coredns v1.6.8 in the virtualcluster.
```
# kubectl apply --kubeconfig vc-1.kubeconfig -f config/sampleswithspec/coredns.yaml
```

## Testing

First, create a normal nginx Pod and a service `my-nginx` using the nginx Pod as the
endpoint in the virtualcluster. Then you can access `my-nginx`
service from any other tenant Pods in the same namespace using DNS. For example:
```
# kubectl exec $TEST_POD -n $SERVICE_NAMESPACE_IN_SUPER -it /bin/sh

sh-4.4# curl my-nginx.default.svc.cluster.local
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
<style>
    body {
        width: 35em;
        margin: 0 auto;
        font-family: Tahoma, Verdana, Arial, sans-serif;
    }
</style>
</head>
<body>
<h1>Welcome to nginx!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>

<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>

<p><em>Thank you for using nginx.</em></p>
</body>
</html>

```

You can observe that the `my_nginx` service has different cluster IPs in tenant master and super master respectively
and the tenant coredns uses the super master cluster ip for service FQDN translation.
