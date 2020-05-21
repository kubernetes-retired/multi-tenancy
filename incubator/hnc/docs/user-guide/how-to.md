# HNC: How-to
_Part of the [HNC User Guide](README.md)_

This document describes common tasks you might want to accomplish using HNC.

## Table of contents

* [Use hierarchical namespace](#use)
  * [Prepare to use hierarchical namespaces](#use-prepare)
  * [Create a subnamespace](#use-subns-create)
  * [Inspect namespace hierarchies](#use-inspect)
  * [Propagating policies across namespaces](#use-propagate)
  * [Select namespaces based on their hierarchies](#use-select)
  * [Delete a subnamespace](#use-subns-delete)
  * [Organize full namespaces into a hierarchy](#use-full)
* [Administer HNC](#admin)
  * [Install or upgrade HNC on a cluster](#admin-install)
  * [Uninstall HNC from a cluster](#admin-uninstall)
  * [Administer who has access to HNC properties](#admin-access)
  * [Modify the object types propagated by HNC](#admin-types)
  * [Gather metrics](#admin-metrics)

<a name="use"/>

## Use hierarchical namespaces

<a name="use-prepare"/>

### Prepare to use hierarchical namespaces as a user

It is possible to interact with hierarchical namespaces purely through
Kubernetes tools such as `kubectl`. However, the `kubectl-hns`
[plugin](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)
greatly simplifies several tasks. This guide illustrates both methods, but we
recommend installing the `kubectl-hns` plugin as well.

To install the plugin, follow the directions below (Linux only):

```
# Select the HNC version that matches the version installed on your cluster.
# Ask your cluster administrator if you're not sure. The latest version is
# shown below.
HNC_VERSION=0.3.0

# Decide where to install the plugin. It just needs to be on your PATH.
PLUGIN_DIR=<any directory on your PATH>

# Download the plugin
curl -L https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/kubectl-hns -o ${PLUGIN_DIR}/kubectl-hns

# Make the plugin executable.
chmod +x ${PLUGIN_DIR}/kubectl-hns

# Ensure the plugin is working
kubectl hns
# The help text should be displayed
```

<a name="use-subns-create">

### Create a subnamespace

In order to create a [subnamespace](concepts.md#basic-subns) of another
namespace, you must have permissions to create the
`subnamespaceanchor.hnc.x-k8s.io` resource in that namespace. Ask your cluster
administrator to give you this permission if you do not have it.

To create a subnamespace “child” underneath parent “parent” using the kubectl
plugin:

```
$ kubectl hns create child -nparent
```

This creates an object called a subnamespace anchor in the parent namespace. HNC
detects that this anchor has been added, and creates the subnamespace for you.

To verify that this worked:

```
$ kubectl hns tree parent
# Output:
parent
└── child
```

To create a subnamespace without the plugin, create the following resource:

```
$ kubectl apply -f - <<EOF
apiVersion: hnc.x-k8s.io/v1alpha1
kind: SubnamespaceAnchor
metadata:
  namespace: parent
  name: child
EOF
```

To verify that this has worked (see comments):

```
$ kubectl get ns child
# Output:
NAME   STATUS   AGE
child  Active   1m

$ kubectl get -oyaml -nparent hns child
# Output:
apiVersion: hnc.x-k8s.io/v1alpha1
kind: SubnamespaceAnchor
metadata:
  name: child
  namespace: default
  … < other stuff > …
status:
  status: ok # <--- This will be something other than 'ok' if there's a problem
```

You can also look inside the new namespace to confirm its set up correctly:

```
$ kubectl get -oyaml -nchild hierarchyconfiguration hierarchy
# Output:
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HierarchyConfiguration
metadata:
  name: hierarchy
  namespace: child
  … < other stuff > …
spec:
  parent: default # <--- this should be the namespace of the anchor
status: {}
```

<a name="use-inspect"/>

### Inspect namespace hierarchies

This section is under construction (as of Map 2020). TL;DR: `kubectl hns tree <name>` and `kubectl hns describe <name>`.

TODO: explain conditions (eg get HNC to try to propagate a `cluster-admin` rolebinding).

<a name="use-propagate"/>

### Propagating policies across namespaces

By default, HNC propagates RBAC `Role` and `RoleBinding` objects. If you create
objects of these kinds in a parent namespace, it will automatically be copied
into any descendant namespaces as well. You cannot modify these propagated
copies; HNC’s admission controllers will attempt to stop you from editing them,
and if you bypass the controllers, HNC will overwrite them.

HNC can also propagate objects other than RBAC objects, but only cluster
administrators can modify this. See [here](#admin-types) for instructions.

<a name="use-select"/>

### Select namespaces based on their hierarchies

HNC inserts labels onto your namespaces to allow trees (and subtrees) of
namespaces to be selected by policies such as `NetworkPolicy`.

This section is under construction (as of Apr 2020). For now, please see
[this demo](https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64/edit#heading=h.9j9w6spvzkdn).

<a name="use-subns-delete"/>

### Delete a subnamespace

In order to delete a subnamespace, you must have permissions to delete its
anchor in its parent namespace. Ask your cluster administrator to give you this
permission if you do not have it.

**WARNING: the protections described in this section only work on clusters with
Kubernetes 1.15 and higher installed. See [issue
#688](https://github.com/kubernetes-sigs/multi-tenancy/issues/688) for details.
In Kubernetes 1.14 and earlier, HNC is unable to stop you from deleting
namespaces.**

You cannot delete a subnamespace by deleting its namespace:

```
$ kubectl delete namespace child
# Output:
Error from server (Forbidden): admission webhook "vnamespace.k8s.io" denied the request: The namespace "child" is a subnamespace. Please delete the subnamespace anchor from the parent namespace "parent" instead.
```

However, if you simply try to delete the subnamespace anchor, it will give you a validation error:

```
$ kubectl delete subns -nparent child
# Output:
Error from server (Forbidden): admission webhook "subnamespaces.hnc.x-k8s.io" denied the request: The subnamespace child doesn't allow cascading deletion. Please set allowCascadingDelete flag first.
```

_Note: `subns` is a short form of `subnamespaceanchor`._

Similarly, if you try to delete the parent of a subnamespace, you’ll get a
validation error, even if the parent is not a subnamespace itself:

```
$ kubectl delete namespace parent
# Output:
Error from server (Forbidden): admission webhook "vnamespace.k8s.io" denied the request: Please set allowCascadingDelete first either in the parent namespace or in all the subnamespaces.
 Subnamespace(s) without allowCascadingDelete set: [child].
```

These errors are there for your protection. Deleting namespaces is very
dangerous, and deleting _subnamespaces_ can result in entire subtrees of
namespaces being deleted as well. Therefore, you may set the
`allowCascadingDelete` field either on the child namespace, on its parent, or (if
the parent is a subnamespace itself) on its parent, and so on. For example, set
the field on `child` if you’re only trying to delete the child, or on `parent` if
you’re trying to delete the parent.

> **WARNING: this option is very dangerous, so you should only set it on the lowest
possible level of the hierarchy.**

> **WARNING: any subnamespaces of the namespace you are deleting will also be
deleted, and so will any subnamespaces of those namespaces, and so on. However,
any full namespaces that are descendants of a subnamespace will not be
deleted.**

> _Note: The inheritance of `allowCascadingDelete` is actually a bit more
> restricted than what's sketched out above: its value is only inherited through
> ancestor _subnamespaces_, or up to the _first_ full namespace._
>
> _For example, if subnamespace `child` has ancestors `parent` and `grandparent`,
> both of which are full namespaces, we will only respect the
> `allowCascadingDelete` field on `parent`, not on `grandparent`. This is
> because if `grandparent` is deleted, `parent` will not be affected, and
> therefore neither will `child`._
>
> _This behaviour may be simplified in a future release of HNC
> ([#730](https://github.com/kubernetes-sigs/multi-tenancy/issues/730))._

To set the `allowCascadingDelete` field on a namespace using the plugin:

```
$ kubectl hns set child --allowCascadingDelete
# Output:
Allowing cascading deletion on 'child'
Succesfully updated 1 property of the hierarchical configuration of child

$ kubectl delete subns child -nparent
# Output:
subnamespaceanchor.hnc.x-k8s.io "child" deleted
```

To set the `allowCascadingDelete` field without the plugin, simply set the
`spec.allowCascadingDelete field` to true in the child’s
`hierarchyconfiguration/hierarchy` object - for example, via:

```
$ kubectl edit -nchild hierarchyconfiguration hierarchy
```

<a name="use-full"/>

### Organize full namespaces into a hierarchy

Most users will only interact with HNC’s hierarchy through subnamespaces. But
you can also organize full namespaces - that is, any Kubernetes namespace that
is _not_ a subnamespace - into hierarchies as well. To do this, you need the
“update” permission for HierarchyConfiguration objects on various namespaces, as
will be described below. If you have the “update” permission of these objects,
we call you an [administrator](concepts.md#admin-admin) of the namespace.

Imagine that `ns-bar` and `ns-foo` are two full namespaces that you created via
kubectl create namespace, and you’d like `ns-foo` to be the parent of `ns-bar`. To
do this using the kubectl plugin:

```
$ kubectl hns set ns-bar --parent ns-foo
```

To do this without the plugin, in `ns-bar`, edit the
`hierarchyconfiguration/hierarchy` object and set its `.spec.parent` field to
`ns-foo`.

In order to make this change, you need to be an administrator of both `ns-foo` and
`ns-bar`. You need to be an admin for `ns-bar` since you’re changing its hierarchy,
and of `ns-foo` because your namespace may start to inherit sensitive properties
such as secrets from `ns-foo`.

If you decide that you no longer want `ns-bar` to be a child of `ns-foo`, you
can do this as follows using the plugin:

```
$ kubectl hns set ns-bar --root
```

To do this without the plugin, in `ns-bar`, edit the
`hierarchyconfiguration/hierarchy` object and delete its `.spec.parent` field.

In this example, `ns-foo` and `ns-bar` were both originally root namespaces - that
is, they did not have any ancestors. However, if `ns-bar` had any ancestors, this
changes the permissions you require to change the parent of `ns-bar` to `ns-foo`:

* **If `ns-bar` and `ns-foo` both have ancestors, but none in common:** In effect,
  you are moving `ns-bar` out of one tree, and into another one. Put another
  way, anyone who was an administrator of the old tree will lose access to
  `ns-bar`. As a result, you must be an administrator of the “oldest” ancestor
  of `ns-bar` - that is, the root namespace that has `ns-bar` as a descendant.
  In the new tree, you still only need to be an administrator of `ns-foo` (the
  same as before) since you’re still only gaining access to information in that
  namespace.
* **If `ns-bar` and `ns-foo` have an ancestor in common:** In this case, you are
  moving `ns-bar` around inside the tree containing both `ns-bar` and `ns-foo`.
  In this case, you must be an administrator of the most recent ancestor to both
  `ns-bar` and `ns-foo`.

Similarly, if you want to make `ns-bar` a root again, you must be an
administrator of the root namespace that is an ancestor of `ns-bar`, since the
admins of that namespace will lose access to `ns-bar` once it becomes a root.

<a name="admin"/>

## Administer HNC

<a name="admin-install"/>

### Install or upgrade HNC on a cluster

We recommend installing HNC onto clusters running Kubernetes v1.15 or later.
Earlier versions of Kubernetes are missing some admission controller features
that leave us unable to validate certain dangerous operations such as deleting
namespaces (see [#680](https://github.com/kubernetes-sigs/multi-tenancy/issues/680)).

There is no need to uninstall HNC before upgrading it unless specified in the
release notes for that version.

#### Install prerequisites

If you are using HNC v0.3 or earlier, you must install Jetstack's cert-manager
prior to installing HNC. This is not needed for HNC v0.4 or later.


```bash
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
```

Wait for the cert-manager deployments to all become healthy. This can take a few
minutes.

#### Install an official image

The most recent official image is v0.3.0.

```bash
# Set the desired release:
HNC_VERSION=v0.3.0

kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml
```

#### Download the kubectl plugin (Linux only)

The `kubectl-hns` plugin makes most HNC use and administration much easier; we
strongly recommend installing it.

```bash
PLUGIN_DIR=<directory where you keep your plugins - just has to be on your PATH>
curl -L https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/kubectl-hns -o ${PLUGIN_DIR}/kubectl-hns
chmod +x ${PLUGIN_DIR}/kubectl-hns
```

#### Install from source

These instructions assume you are installing on GKE and have a GCR repo. If
you'd like to contribute instructions for other clouds, please do so!

```bash
# The GCP project of the GCR repo:
export PROJECT_ID=my-gcp-project

# A tag for the image you want to build (default is 'latest')
export HNC_IMG_TAG=test-img

# Build and deploy. Note: you need kubebuilder.io installed for this. This will
# also build the kubectl-hns plugin and install it at ${GOPATH}/bin/kubectl-hns;
# please ensure this path is in your PATH env var in order to use it.
make deploy
```


<a name="admin-uninstall"/>

### Uninstall HNC from a cluster

**WARNING:** this will also delete all the hierarchical relationships between
your namespaces. Reinstalling HNC will _not_ recreate these relationships. There
is no need to uninstall HNC before upgrading it unless specified in the release
notes for that version.

```bash
rm ${PLUGIN_DIR}/kubectl-hns
kubectl delete -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml

# Don't need to delete the cert manager if you plan to reinstall it later.
kubectl delete -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
```

<a name="admin-access"/>

### Administer who has access to HNC properties

HNC has three significant objects whose access administrators should carefully
control:

* The `HNCConfig` object. This is a single non-namespaced object (called `config`)
  that defines the behaviour of the entire cluster. It should only be modifiable
  by cluster administrators. In addition, since it may contain information about
  any namespace in the cluster, it should only be readable by users trusted with
  this information.
* The `HierarchyConfiguration` objects. There’s either zero or one of these in
  each namespace, with the name `hierarchy` if it exists. Any user with `update`
  access to this object is known as an [administrator](concepts.md#admin) of
  that namespace and its subtree, and access should be granted carefully as a
  result.
* The `SubnamespaceAnchor` objects. These are used to create subnamespaces.
  Generally speaking, access to _create_ or _read_ these objects should be
  granted quite freely to users to have permission to use other objects in a
  given namespace, since this allows them to use hierarchical namespaces to
  organize their objects. However, be aware that any `ResourceQuota` in the
  parent namespace will not apply to any subnamespaces.

> _Note: There are various projects underway to allow resource quotas to be
> applied to trees of namespaces. For example, see the [Dec 3 2019
> wg-multitenancy meeting](https://www.youtube.com/watch?v=V0Zw82nOAEE). Contact
> wg-multitenancy if you need this feature._

It is important to note that just because a user _created_ a subnamespace, that
does not make them an _administrator_ of that subnamespace. That requires
someone to explicitly grant them the `update` permission for the
`HierarchyConfiguration` object in that namespace. As a result, an unprivileged
user who creates a subnamespace generally can’t delete it as well, since this
would require them to set the `allowCascadingDelete` property of the child
namespace.

<a name="admin-types"/>

### Modify the object types propagated by HNC

HNC supports following propagation modes for each object type:
* propagate (the default): propagates objects from ancestors to descendants and
  deletes obsolete descendants.
* remove: deletes all existing propagated copies.
* ignore: stops modifying this type. New or changed objects will not be
  propagated, and obsolete objects will not be deleted. The `inheritedFrom` label
  is not removed. Any unknown mode is treated as `ignore`.

HNC propagates `Roles` and `RoleBindings` by default. You can also set any type
of Kubernetes resource to any of the propagation modes discussed above. To do
so, you need cluster privileges.

To configure an object type using the kubectl plugin:

```
kubectl hns config set-type --apiVersion apiVersion --kind kind [propagate|remove|ignore]
```

For example:

```
kubectl hns config set-type --apiVersion v1 --kind Secret propagate
```

To verify that this worked:

```
kubectl hns config describe
# Output:
Synchronized types:
* Propagating: Role (rbac.authorization.k8s.io/v1)
* Propagating: RoleBinding (rbac.authorization.k8s.io/v1)
* Propagating: Secret (v1) # <<<< This should be added
```

To configure an object type without using the kubectl plugin, edit the existing
`HNCConfiguration` object (HNC will autocreate it for you when it’s installed):

```
kubectl edit hncconfiguration config
```

Modify the config to include custom type configurations:

```
apiVersion: hnc.x-k8s.io/v1alpha1
kind: HNCConfiguration
metadata:
  name: config
spec:
  types:
    # Spec for other types
    ...
    - apiVersion: v1   <<<
      kind: Secret     <<< This should be added
      mode: propagate  <<<
```

<a name="admin-metrics"/>

### Gather metrics

HNC makes the following metrics available, and can be monitored via Stackdriver
(next section) or Prometheus (experimental - see
[#433](https://github.com/kubernetes-sigs/multi-tenancy/issues/433)).

|Metric                                                |Description   |
|:---------------------------------------------------- |:-------------|
| `hnc/namespace_conditions`                           | The number of namespaces affected by [conditions](concepts.md#admin-conditions), tagged by the condition code and whether or not the conditions are critical or not |
| `hnc/reconcilers/hierconfig/total`                   | The total number of HierarchyConfiguration (HC) reconciliations happened |
| `hnc/reconcilers/hierconfig/concurrent_peak`         | The peak concurrent HC reconciliations happened in the past 60s, which is also the minimum Stackdriver reporting period and the one we're using |
| `hnc/reconcilers/hierconfig/hierconfig_writes_total` | The number of HC writes happened during HC reconciliations |
| `hnc/reconcilers/hierconfig/namespace_writes_total`  | The number of namespace writes happened during HC reconciliations |
| `hnc/reconcilers/object/total`                       | The total number of object reconciliations happened |
| `hnc/reconcilers/object/concurrent_peak`             | The peak concurrent object reconciliations happened in the past 60s, which is also the minimum Stackdriver reporting period and the one we're using |

#### Use Stackdriver on GKE

To view HNC Metrics in Stackdriver, you will need a GKE cluster with HNC
installed and a method to access Cloud APIs, specifically Stackdriver monitoring
APIs, from GKE. We recommend using [Workload
Identity](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity)
to minimize the permissions required to log metrics.  Once it's set up, you can
view the metrics in Stackdriver  [Metrics
Explorer](https://cloud.google.com/monitoring/charts/metrics-explorer) by
searching the metrics keywords (e.g. `namespace_conditions`).

In order to monitor metrics via Stackdriver:
1. Enable Workload Identity (WI) on either a
   [new](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#enable_workload_identity_on_a_new_cluster)
   or
   [existing](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#enable_workload_identity_on_an_existing_cluster)
   cluster.
2. Install HNC as described [above](#admin-install).
3. [Create a Google service account (GSA)](https://cloud.google.com/docs/authentication/production#creating_a_service_account):
    ```bash
    gcloud iam service-accounts create [GSA_NAME]
    ```
4. Grant “[Monitoring Metric Writer](https://cloud.google.com/monitoring/access-control#mon_roles_desc)”
role to the GSA:
    ```bash
    gcloud projects add-iam-policy-binding [PROJECT_ID] --member \
      "serviceAccount:[GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com" \
      --role "roles/monitoring.metricWriter"
    ```
5. Create an [Cloud IAM policy binding](https://cloud.google.com/sdk/gcloud/reference/iam/service-accounts/add-iam-policy-binding)
between `hnc-system/default` KSA and the newly created GSA:
     ```
     gcloud iam service-accounts add-iam-policy-binding \
       --role roles/iam.workloadIdentityUser \
       --member "serviceAccount:[PROJECT_ID].svc.id.goog[hnc-system/default]" \
       [GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com
   ```
6. Add the `iam.gke.io/gcp-service-account=[GSA_NAME]@[PROJECT_ID]` annotation to
the KSA, using the email address of the Google service account:
     ```
     kubectl annotate serviceaccount \
       --namespace hnc-system \
       default \
       iam.gke.io/gcp-service-account=[GSA_NAME]@[PROJECT_ID].iam.gserviceaccount.com
   ```

If everything is working properly, you should start to see metrics in the
Stackdriver metrics explorer from HNC. Otherwise, you can inspect the service
account configuration by creating a Pod with the Kubernetes service account that
runs the `cloud-sdk` container image, and connecting to it with an interactive
session:

```
kubectl run --rm -it \
  --generator=run-pod/v1 \
  --image google/cloud-sdk:slim \
  --serviceaccount default \
  --namespace hnc-system \
  workload-identity-test

# Inside the new pod:

gcloud auth list
```
