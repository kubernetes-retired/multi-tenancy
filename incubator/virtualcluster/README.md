# Virtualcluster - Enabling Kubernetes Hard Multi-tenancy

Virtualcluster represents a new architecture to address various Kubernetes control plane isolation challenges.
It extends existing namespace based Kubernetes multi-tenancy model by providing each tenant a cluster view.
Virtual cluster completely leverages Kubernetes extendability and preserves full API compatibility.
That being said, the core Kubernetes components are not modified in virtual cluster.

In virtualcluster, each tenant is assigned a dedicated tenant master, which is a vanilla Kubernetes.
Tenant can create cluster scope resources such as namespaces and CRDs in tenant master without affecting others.
As a result, most of the isolation problems due to sharing one apiserver disappear.
The Kubernetes cluster that manages the actual physical nodes is called a super master, which now
becomes a Pod resource provider. Virtualcluster is composed of the following components:

- **vc-manager**: A new CRD [virtualcluster](pkg/apis/tenancy/v1alpha1/virtualcluster_types.go) is introduced
to model the tenant master. vc-manager manages the lifecycle of each virtualcluster custom resource.
Based on the specification, it either creates apiserver and controller-manager Pods in local K8s cluster,
or imports an existing cluster if its valid kubeconfig is provided.

- **syncer**: A centralized controller that populates API objects needed for Pod provision from every tenant master
to the super master, and update the object statuses back. It also periodically scans the synced objects to ensure
the states between tenant master and super master are consistent.

- **vn-agent**: A node daemon that proxies all tenant kubelet API requests to the kubelet process that running
in the node. It ensures each tenant can only access its own Pods in the node.

With all above, from the tenant’s perspective, each tenant master behaves like an intact Kubernetes with nearly
full API capabilities.

## Live Demos

The below are two demos that illustrate the use of a virtualcluster.
The short demo is created by [@zhuangqh](https://github.com/zhuangqh) for an introduction. A
detailed in-depth demo is also provided which is a video recording from a WG bi-weekly meeting.

Short (~5 mins) | Long (~50 mins) 
--- | --- 
[![](http://img.youtube.com/vi/QvpNehTNRyk/0.jpg)](http://www.youtube.com/watch?v=QvpNehTNRyk "vc-demo-short") | [![](http://img.youtube.com/vi/Kow00IEUbAA/0.jpg)](http://www.youtube.com/watch?v=Kow00IEUbAA "vc-demo-long")

## Quick Start

Please follow the [instructions](./doc/demo.md) to install virtualcluster in your local K8s cluster.

## Supported/Not Supported

Virtualcluster passes most of the Kubernetes conformance tests. One failed test asks for supporting
`subdomain` which cannot be easily done in the virtualcluster architecture. There are other considerations
that users should be aware of:

- Virtualclsuter follows a serverless design pattern. The super master node topology is not fully exposed in
tenant master. Only the nodes that tenant Pods are running on will be shown in tenant master. As a result,
virtualcluster does not support DaemonSet alike workloads in tenant master. In other words, the syncer controller
rejects a newly created tenant Pod if its `nodename` has been set in the spec.

- It is recommended to increase the tenant master node controller `--node-monitor-grace-period` parameter to a larger value
( >60 seconds, done in the sample clusterversion [yaml](config/sampleswithspec/clusterversion_v1_nodeport.yaml) already).
The syncer controller does not update the node lease objects in tenant master,
hence the default grace period is too small.

- Coredns is not tenant-aware. Hence, tenant should install coredns in tenant master if DNS is required. 
The DNS service should be created in `kube-system` namespace using name `kube-dns`. The syncer controller can then
recognize the DNS service cluster IP in super master and inject it into Pod spec `dnsConfig`.

- Since coredns is installed in tenant master, the service cluster IPs have to be identical in tenant
master and super master respectively so that coredns can provide correct cluster IP translation for service cname.
This requires both clusters to have the same CIDR.

- Virtualcluster fully support tenant service account.

- Virtualclsuter does not support tenant PersistentVolumes. All PVs and Storageclasses are provided by the super master.

- It is recommended that tenant master and super master should use the same Kubernetes version to avoid
incompatible API behaviors. The syncer controller and vn-agent are built using Kubernetes 1.16 APIs, hence
higher Kubernetes versions are not supported at this moment.

## Release

The first release is coming soon.

## Community
Virtualcluster is a SIG multi-tenancy WG incubator project.
If you have any questions or want to contribute, you are welcome to file issues or pull requests.

You can also directly contact virtualcluster maintainers via the WG [slack channel](https://kubernetes.slack.com/messages/wg-multitenancy).

Lead developer: @Fei-Guo(f.guo@alibaba-inc.com)
