# Virtualcluster - Enabling Kubernetes Hard Multi-tenancy 

Virtualcluster represents a new architecture to address various Kubernetes control plane isolation challenges.
It extends existing namespace based Kubernetes multi-tenancy model by providing each tenant a cluster view.
Virtual cluster completely leverages Kubernetes extendability and preserves full API compatibility. 
That being said, the core Kubernetes components are not modified in virtual cluster.

In virtualcluster, each tenant is assigned a dedicated tenant master, which is a vanilla Kubernetes.
Tenant can create cluster scope resources such as namespaces and CRDs in tenant master without affecting others.
As a result, most of the isolation problems due to sharing one apiserver disappear. 
The Kubernetes cluster that manages the actual physical nodes is called the super master, which now 
becomes a Pod resource provider. Virtual cluster is composed of the following components:

- **vc-manager**: A CRD controller that manage the lifecycle of virtualcluster custom resources. It currently supports
two modes: native mode and cloud mode, to provision tenant master components. In native mode, vc-manager creates
apiserver and controller-manager Pods in local K8s cluster, and in cloud mode, vc-manager calls public APIs to create
master components in public cloud.

- **syncer**: A centralized controller that populates API objects needed for Pod provision from every tenant master
to the super master, and update the object status back. It also periodically scans the synced objects to ensure 
the states between tenant master and super master are consistent.

- **vn-agent**: A node daemon that proxies all tenant kubelet API requests to the kubelet process that running
in the node. It ensures each tenant can only access its own Pods in the node.

With all above, from the tenant’s perspective, each tenant master behaves like an intact Kubernetes with nearly 
full API capabilities. 

## A Short Demo

The below is a short demo created by [@zhuangqh](https://github.com/zhuangqh) which illustrates the high level
usage of a virtualcluster.

[![](http://img.youtube.com/vi/QvpNehTNRyk/0.jpg)](http://www.youtube.com/watch?v=QvpNehTNRyk "vc-demo")

## Quick Start

Please follow the [instructions](./doc/demo.md) to install virtualcluster in your local K8s cluster.

## Supported/Not Supported

Virtualcluster passes most of the Kubernetes conformance tests. One failed test asks for supporting 
`subdomain` which cannot be easily done in virtualcluster architecture. There are other considerations
that users should be aware of: 


- Virtualclsuter follows a serverless design pattern. The super master node topology is not fully exposed in
each tenant master. Only the nodes that run tenant Pods are shown in the tenant master. As a result, 
virtualcluster does not support DaemonSet like workload in tenant master. In other words, the syncer controller 
rejects a newly created tenant Pod if its `nodename` has been set in the spec.

- It is recommended to increase the tenant master node controller `--node-monitor-grace-period` parameter to a larger value 
( >60 seconds, done in sample clusterversion [yaml](config/sampleswithspec/clusterversion_v1_nodeport.yaml) already). 
The syncer controller does not update the node lease objects in tenant master, 
hence the default grace period is too small.

- Coredns is not tenant-aware. Hence, tenant should install coredns in tenant master if DNS is required. 
The DNS service should be created in tenant master `kube-system` namespace using name `kube-dns`. The syncer controller can then
recognize the DNS service cluster IP and inject it into Pod spec `dnsConfig`.

- To fully support service in virtualcluster, the tenant master CIDR and super master CIDR should be the same.

- Virtualcluster fully support tenant service account.

- Virtualclsuter does not support tenant PersistentVolumes. All PVs and Storageclasses are provided by the super master. 

- It is recommended that tenant masters and super master use the same Kubernetes version to avoid
incompatible API behaviors. The syncer controller and vn-agent are built using Kubernetes 1.16 APIs, hence
higher Kubernetes versions are not supported.


## Release

The first release is coming soon.

## Community
Virtualcluster is a SIG multi-tenancy WG incubator project. 
If you have any questions or want to contribute, you are welcome to file issues or pull requests.

You can also directly contact virtualcluster maintainers via the WG [slack channel](https://kubernetes.slack.com/messages/wg-multitenancy).

Lead developer: @Fei-Guo(f.guo@alibaba-inc.com)

