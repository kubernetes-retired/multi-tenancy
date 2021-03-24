# VirtualCluster - Enabling Kubernetes Hard Multi-tenancy

VirtualCluster represents a new architecture to address various Kubernetes control plane isolation challenges.
It extends existing namespace based Kubernetes multi-tenancy model by providing each tenant a cluster view.
VirtualCluster completely leverages Kubernetes extendability and preserves full API compatibility.
That being said, the core Kubernetes components are not modified in virtual cluster.

With VirtualCluster, each tenant is assigned a dedicated tenant control plane, which is a upstream Kubernetes distribution.
Tenants can create cluster scope resources such as namespaces and CRDs in the tenant control plane without affecting others.
As a result, most of the isolation problems due to sharing one apiserver disappear.
The Kubernetes cluster that manages the actual physical nodes is called a super cluster, which now
becomes a Pod resource provider. VirtualCluster is composed of the following components:

- **vc-manager**: A new CRD [VirtualCluster](pkg/apis/tenancy/v1alpha1/virtualcluster_types.go) is introduced
to model the tenant control plane. `vc-manager` manages the lifecycle of each `VirtualCluster` custom resource.
Based on the specification, it either creates `apiserver`, `etcd` and `controller-manager` Pods in local K8s cluster,
or imports an existing cluster if a valid `kubeconfig` is provided.

- **syncer**: A centralized controller that populates API objects needed for Pod provisioning from every tenant control plane
to the super cluster, and bidirectionally syncs the object statuses. It also periodically scans the synced objects to ensure
the states between tenant control plane and super cluster are consistent.

- **vn-agent**: A node daemon that proxies all tenant kubelet API requests to the kubelet process that running
in the node. It ensures each tenant can only access its own Pods in the node.

With all above, from the tenant’s perspective, each tenant control plane behaves like an intact Kubernetes with nearly full API capabilities.
For more technical details, please check our [ICDCS 2021 paper.](./doc/vc-icdcs.pdf) 

## Live Demos/Presentations

Kubecon EU 2020 talk (~25 mins) | WG meeting demo (~50 mins)
--- | ---
[![](http://img.youtube.com/vi/5RgF_dYyvEY/0.jpg)](https://www.youtube.com/watch?v=5RgF_dYyvEY "vc-kubecon-eu-2020") | [![](http://img.youtube.com/vi/Kow00IEUbAA/0.jpg)](http://www.youtube.com/watch?v=Kow00IEUbAA "vc-demo-long")

## Quick Start

Please follow the [instructions](./doc/demo.md) to install VirtualCluster in your local K8s cluster.

## Abstraction

In VirtualCluster, tenant control plane owns the source of the truth for the specs of all the synced objects. 
The exceptions are persistence volume, storage class and priority class resources whose source of the truth is the super cluster.
The syncer updates the synced object's status in each tenant control plane, 
acting like a regular resource controller. This abstraction model means the following assumptions:
- The synced object spec _SHOULD_ not be altered by any arbitrary controller in the super cluster.
- Tenant master owns the lifecycle management for the synced object. The synced objects _SHOULD NOT_ be
  managed by any controllers (e.g., StatefulSet) in the super cluster.

If any of the above assumptions is violated, VirtualCluster may not work as expected. Note that this 
does not mean that a cluster administrator cannot install webhooks, for example, a sidecar webhook, 
in the super cluster. Those webhooks will still work but the changes are going
to be hidden to tenants. Alternatively, those webhooks can be installed in tenant control planes so that
tenants will be aware of all changes.

## Limitations

Ideally, tenants should not be aware of the existence of the super cluster in most cases. 
There are still some noticeable differences comparing a tenant control plane and a normal Kubernetes cluster.

- In the tenant control plane, node objects only show up after tenant Pods are created. The super cluster
  node topology is not fully exposed in the tenant control plane. This means the VirtualCluster does not support
  `DaemonSet` alike workloads in tenant control plane. Currently, the syncer controller rejects a newly
  created tenant Pod if its `nodename` has been set in the spec. 

- The syncer controller manages the lifecycle of the node objects in tenant control plane but
  it does not update the node lease objects in order to reduce network traffic. As a result,
  it is recommended to increase the tenant control plane node controller `--node-monitor-grace-period` 
  parameter to a larger value ( >60 seconds, done in the sample clusterversion
  [yaml](config/sampleswithspec/clusterversion_v1_nodeport.yaml) already).

- Coredns is not tenant-aware. Hence, tenant should install coredns in the tenant control plane if DNS is required.
The DNS service should be created in the `kube-system` namespace using the name `kube-dns`. The syncer controller can then
recognize the DNS service's cluster IP in super cluster and inject it into any Pod `spec.dnsConfig`.

- The cluster IP field in the tenant service spec is a bogus value. If any tenant controller requires the
actual cluster IP that takes effect in the super cluster nodes, a special handling is required. 
The syncer will backpopulate the cluster IP used in the super cluster in the 
annotations of the tenant service object using `transparency.tenancy.x-k8s.io/clusterIP` as the key.
Then, the workaround usually is going to be a simple code change in the controller. 
This [document](./doc/tenant-dns.md) shows an example for coredns.

- VirtualCluster does not support tenant PersistentVolumes. All PVs and Storageclasses are provided by the super cluster.

VirtualCluster passes most of the Kubernetes conformance tests. One failing test asks for supporting
`subdomain` which cannot be easily done in the VirtualCluster.

## FAQ

### Q: What is the difference between VirtualCluster and multi-cluster solution?

One of the primary design goals of VirtualCluster is to improve the overall resource utilization
of a super cluster by allowing multiple tenants to share the node resources in a control plane isolated manner. 
A multi-cluster solution can achieve the same isolation goal but resources won't be shared causing
nodes to have lower utilization.

### Q: Can the tenant control plane run its own scheduler?

VirtualCluster was primarily designed for serverless use cases where users normally do not have
scheduling preferences. Using the super cluster scheduler can much easily
achieve good overall resource utilization. For these reasons, 
VirtualCluster does not support tenant scheduler. It is technically possible
to support tenant scheduler by exposing some of the super cluster nodes directly in
tenant control plane. Those nodes have to be dedicated to the tenant to avoid any scheduling
conflicts. This type of tenant should be exceptional.

### Q: What is the difference between Syncer and Virtual Kubelet? 

They have similarities. In some sense, the syncer controller can be viewed as the replacement of a virtual
kubelet in cases where the resource provider of the virtual kubelet is a Kubernetes cluster. The syncer 
maintains the one to one mapping between a virtual node in tenant control plane and a real node
in the super cluster. It preserves the Kubernetes API compatibility as closely as possible. Additionally, 
it provides fair queuing to mitigate tenant contention.

## Release

The first release is coming soon.

## Community
VirtualCluster is a SIG multi-tenancy WG incubator project.
If you have any questions or want to contribute, you are welcome to file issues or pull requests.

You can also directly contact VirtualCluster maintainers via the WG [slack channel](https://kubernetes.slack.com/messages/wg-multitenancy).

Lead developer: @Fei-Guo(f.guo@alibaba-inc.com)
