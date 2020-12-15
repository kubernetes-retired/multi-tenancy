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
  * [Resolve conditions on a namespace](#use-resolve-cond)
  * [Limit the propagation of an object to descendant namespaces](#use-limit-propagation)
* [Administer HNC](#admin)
  * [Install or upgrade HNC on a cluster](#admin-install)
  * [Uninstall HNC from a cluster](#admin-uninstall)
  * [Backing up and restoring HNC data](#admin-backup-restore)
  * [Administer who has access to HNC properties](#admin-access)
  * [Modify the resources propagated by HNC](#admin-resources)
  * [Gather metrics](#admin-metrics)
  * [Modify command-line arguments](#admin-cli-args)

<a name="use"/>

## Use hierarchical namespaces

<a name="use-prepare"/>

### Prepare to use hierarchical namespaces as a user

It is possible to interact with hierarchical namespaces purely through
Kubernetes tools such as `kubectl`. However, the `kubectl-hns`
[plugin](https://kubernetes.io/docs/tasks/extend-kubectl/kubectl-plugins/)
greatly simplifies several tasks. This guide illustrates both methods, but we
recommend installing the `kubectl-hns` plugin.

You can install the plugin by following the instructions for the [latest
release](https://github.com/kubernetes-sigs/multi-tenancy/releases/tag/hnc-v0.7.0).

<a name="use-subns-create">

### Create a subnamespace

In order to create a [subnamespace](concepts.md#basic-subns) of another
namespace, you must have permissions to create the
`subnamespaceanchor.hnc.x-k8s.io` resource in that namespace. Ask your cluster
administrator to give you this permission if you do not have it.

To create a subnamespace “child” underneath parent “parent” using the kubectl
plugin:

```
$ kubectl hns create child -n parent
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
apiVersion: hnc.x-k8s.io/v1alpha2
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

$ kubectl get -oyaml -nparent subns child
# Output:
apiVersion: hnc.x-k8s.io/v1alpha2
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
apiVersion: hnc.x-k8s.io/v1alpha2
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

To get an overview of the hierarchy of your entire cluster, use one of the
following variants of the `tree` command:

```bash
kubectl hns tree --all-namespaces
kubectl hns tree -A
```

You can also limit this display to a single subtree via:

```bash
kubectl hns tree ROOT_NAMESPACE
```

In addition to showing you the structure of your hierarchy, it will also give
you high-level information on any problems with the hierarchies, known as
[conditions](concepts.md#admin-conditions).

For detailed information on any one namespace, including:

* Its children
* Its conditions
* Any HNC problems with objects in the namespace

Use the more detailed `describe` command:

```bash
kubectl hns describe NAMESPACE
```

<a name="use-propagate"/>

### Propagating policies across namespaces

By default, HNC propagates RBAC `Role` and `RoleBinding` objects. If you create
objects of these kinds in a parent namespace, it will automatically be copied
into any descendant namespaces as well. You cannot modify these propagated
copies; HNC’s admission controllers will attempt to stop you from editing them.

Similarly, if you try to create an object in a parent ancestor with the same
name as an object in one of its descendants, HNC will stop you from doing so,
because this would result in the objects in the descendants being silently
overwritten. HNC will also prevent you from changing the parent of a namespace
if this would result in objects being overwritten.

However, if you bypass these admission controllers - for example, by updating
objects while HNC is being upgraded - HNC _will_ overwrite conflicting objects
in descendant namespaces. This is to ensure that if you are able to successfully
create a policy in an ancestor namespace, you can be confident that it will be
uniformly applied to all descendant namespaces.

HNC can also propagate objects other than RBAC objects, but only cluster
administrators can modify this. See [here](#admin-resources) for instructions.

Occasionally, objects might fail to be propagated to descendant namespaces for a
variety of reasons - e.g., HNC itself might not have sufficient RBAC
permissions. To understand why an object is not being propagated to a namespace,
use `kubectl hns describe <ns>`, where `<ns>` is either the source (ancestor) or
destination (descendant) namespace.

<a name="use-select"/>

### Select namespaces based on their hierarchies

HNC inserts labels onto your namespaces to allow trees (and subtrees) of
namespaces to be selected by policies such as `NetworkPolicy`.

This section is under construction (as of Oct 2020). For now, please see the
[quickstart](quickstart.md#netpol).

<a name="use-subns-delete"/>

### Delete a subnamespace

In order to delete a subnamespace, you must first have permissions to delete its
anchor in its parent namespace. Ask your cluster administrator to give you this
permission if you do not have it.

Subnamespaces are _always_ manipulated via their anchors. For example, you
cannot delete a subnamespace by deleting it directly:

```
$ kubectl delete namespace child
# Output:
Error from server (Forbidden): admission webhook "namespaces.x-hnc.k8s.io" denied the request: The namespace "child" is a subnamespace. Please delete the subnamespace anchor from the parent namespace "parent" instead.
```

Instead, you must delete its anchor (note that `subns` is a short form of
`subnamespaceanchor`):

```
$ kubectl delete subns child -n parent
```

This _seems_ to imply that if you delete a _parent_ namespace, all its
subnamespace children (and their descendants) will be deleted as well, since all
objects in a namespace (such as anchors) are deleted along with the namespace.
However, if you actually try this, you'll get an error:

```
$ kubectl delete namespace parent
# Output:
Error from server (Forbidden): admission webhook "namespaces.hnc.x-k8s.io" denied the request: Please set allowCascadingDeletion first either in the parent namespace or in all the subnamespaces.
 Subnamespace(s) without allowCascadingDeletion set: [child].
```

These errors are there for your protection. Deleting namespaces is very
dangerous, and deleting _subnamespaces_ can result in entire subtrees of
namespaces being deleted as well. Therefore, if deleting a namespace (or
subnamespace) would result in the deletion of any namespace _other than the one
explicitly being deleted_, HNC requires that you must specify the
`allowCascadingDeletion` field on either all the namespaces that will be
implicitly deleted, or any of their ancestors.

The `allowCascadingDeletion` field is a bit like `rm -rf` in a Linux shell.

> **WARNING:** this option is very dangerous, so you should only set it on the
> lowest possible level of the hierarchy.

> **WARNING:** any subnamespaces of the namespace you are deleting will also be
> deleted, and so will any subnamespaces of those namespaces, and so on.
> However, any _full_ namespaces that are descendants of a subnamespace will not
> be deleted.

To set the `allowCascadingDeletion` field on a namespace using the plugin:

```
$ kubectl hns set parent --allowCascadingDeletion
# Output:
Allowing cascading deletion on 'parent'
Succesfully updated 1 property of the hierarchical configuration of parent

$ kubectl delete namespace parent
# Should succeed
```

To set the `allowCascadingDeletion` field without the plugin, simply set the
`spec.allowCascadingDeletion field` to true in the namespace's
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

<a name="use-resolve-cond"/>

### Resolve conditions on a namespace

If the namespace has the following condition:
```
ActivitiesHalted (ParentMissing): Parent "<namespace>" does not exist
```
It means that this namespace is orphaned and its parent has been deleted. To fix
 this, you need to either create the parent, or mark this namespace as a root
 namespace by using:
```
$ kubectl hns set --root <namespace>
```

<a name="use-limit-propagation"/>

### Limit the propagation of an object to descendant namespaces

***Exceptions are only available in HNC v0.7***

To limit the propagation of an object, annotate it with an ***exception***. You
can use any of the following annotations:

* **`propagate.hnc.x-k8s.io/select`**: The object will only be propagated to
  namespaces whose labels match the label selector. The value for this selector
  has to be a [valid Kubernetes label
  selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors).

* **`propagate.hnc.x-k8s.io/treeSelect`**: Use a single namespace name to
  represent where this object should be propagated, or use a comma-separated
  list of negated (“!”) namespaces to represent which namespaces not to
  propagate to (e.g. `!child1, !child2` means do not propagate to `child1` and
  `child2`). For example, this can be used to propagate an object to a child
  namespace, but not a grand-child namespace, using the value `child,
  !grand-child`.

* **`propagate.hnc.x-k8s.io/none`**: Setting `none` to `true` (case insensitive)
  will result in the object not propagating to _any_ descendant namespace. Any
  other value will be rejected.

For example, consider a case with a parent namespace with three child
namespaces, and the parent namespace has a secret called `my-secret`. To set
`my-secret` propagate to `child1` namespace (but nothing else), you can use:

```bash
kubectl annotate secret my-secret -n parent propagate.hnc.x-k8s.io/treeSelect=child1
# OR
kubectl annotate secret my-secret -n parent propagate.hnc.x-k8s.io/treeSelect="!child2, !child3"
# OR
kubectl annotate secret my-secret -n parent propagate.hnc.x-k8s.io/select=child1.tree.hnc.x-k8s.io/depth
# OR
kubectl annotate secret my-secret -n parent propagate.hnc.x-k8s.io/select="!child2.tree.hnc.x-k8s.io/depth, !child3.tree.hnc.x-k8s.io/depth"
```

To set `my-secret` not to propagate to any namespace, you can use:

```bash
kubectl annotate secret my-secret -n parent propagate.hnc.x-k8s.io/none=true
```

All these are equivalent to creating the object with the selector annotations:

```bash
cat << EOF | kubectl create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    propagate.hnc.x-k8s.io/treeSelect: child1
  name: my-secret
  namespace: parent
... other fields ...
EOF
```

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

#### Install an official release and the kubectl plugin

[The most recent official release is
v0.7.0](https://github.com/kubernetes-sigs/multi-tenancy/releases/tag/hnc-v0.7.0).
Please see that page for release notes and installation instructions.

#### Install from source

These instructions assume you are installing on GKE and have a GCR repo. If
you'd like to contribute instructions for other clouds, please do so!

```bash
# The GCP project of the GCR repo:
export PROJECT_ID=my-gcp-project

# A tag for the image you want to build (default is 'latest')
export HNC_IMG_TAG=test-img

# Build and deploy to the cluster identified by your current kubectl context. This will
# also build the kubectl-hns plugin and install it at # ${GOPATH}/bin/kubectl-hns;
# please ensure this path is in your PATH env var in order to use it.
make deploy
```

<a name="admin-uninstall"/>

### Uninstall HNC from a cluster

To temporarily disable HNC, simply delete its deployment and webhooks:

```bash
kubectl -n hnc-system delete deployment hnc-controller-manager
kubectl delete validatingwebhookconfiguration.admissionregistration.k8s.io hnc-validating-webhook-configuration
```

You may also completely delete HNC, including its CRDs and namespaces. However,
**this is a destructive process that results in some data loss.** In particular,
you will lose any cluster-wide configuration in your `HNCConfiguration` object,
as well as any hierarchical relationships between different namespaces,
_excluding_ subnamespaces (subnamespace relationships are saved as annotations
on the namespaces themselves, and so can be recreated when HNC is reinstalled).

To avoid data loss, consider [backing up](#admin-backup-restore) your HNC
objects so they can later be restored.

Note that even though the subnamespace anchors are deleted during this process,
the namespaces themselves will not be. HNC distinguishes between anchors that
are being deleted "normally" and those that are being deleted because their CRD
is being removed.

To completely delete HNC, including all non-subnamespace hierarchical
relationships and configuration settings:

```bash
# Firstly, delete the CRDs. Some of the objects have finalizers on them, so
# if you delete the deployment first, the finalizers will never be removed
# and you won't be able to delete the objects without explicitly removing
# the finalizers first.
kubectl get crds | grep .hnc.x-k8s.io | awk '{print $1}' | xargs kubectl delete crd

# Delete the rest of HNC.
kubectl delete -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml
```

<a name="admin-backup-restore"/>

### Backing up and restoring HNC data

If you need to [completely uninstall HNC](#admin-uninstall), but don't want to
lose all your hierarchical data (other than your subnamespaces), you can backup
the data before uninstalling HNC, and restore it afterwards. However, be warned
-- this is a fairly manual and not entirely bulletproof process. As an
alternative, you may wish to consider using an external source of truth ([such
as Git](best-practices.md#gitops)) to store your cluster-wide configuration and
any full namespace hierarchical relationships.

To backup your data, export HNC's custom resources as follows:

```bash
# Save any nonstandard configuration:
kubectl get hncconfiguration config -oyaml > hncconfig.yaml

# Save any explicit hierarchies (not needed if you only have subnamespaces):
kubectl get hierarchyconfigurations -A -oyaml > structure.yaml
```

After HNC is reinstalled, it will recreate the `HierarchicalConfiguration`
objects in every namespace, and may automatically create these objects even in
some full namespaces as well (for example, in the parents of subnamespaces). In
order for your backed-up objects to be restored properly, edit `structure.yaml`
to delete the `.metadata.uid` and `metadata.resourceVersion` fields from each
object **prior** to applying the file.  It may also be convenient to delete the
`.metadata.selfLink` field, which is alphabetically between the other two
fields; this is safe.

Once you are ready, first reapply the cluster-wide configuration via `kubectl
apply -f hncconfig.yaml`, and then the structural relationships via `kubectl
apply -f structure.yaml`.

Finally, resolve any `SubnamespaceAnchorMissing` conditions. Type `kubectl hns
tree -A` to identify all subnamespaces affected, by this condition, and then
recreate recreate the anchors manually by typing `kubectl hns create <subns> -n
<parent>`.

<a name="admin-access"/>

### Administer who has access to HNC properties

HNC has three significant objects whose access administrators should carefully
control:

* The `HNCConfiguration` object. This is a single non-namespaced object (named
  `config`) that defines the behaviour of the entire cluster. It should only be
  modifiable by cluster administrators. In addition, since it may contain
  information about any namespace in the cluster, it should only be readable by
  users trusted with this information. This object is automatically created by
  HNC when it's installed.
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
would require them to set the `allowCascadingDeletion` property of the child
namespace.

<a name="admin-types"/>
<a name="admin-resources"/>

### Modify the resources propagated by HNC

HNC is configured via the [`HNCConfiguration`](#admin-access) object.  You can
inspect this object directly via `kubectl get -oyaml hncconfiguration config`,
or with the HNS plugin via `kubectl hns config describe`.

The most important type of configuration is the way each object type
("resource") is synchronized across namespace hierarchies. This is known as the
"synchronization mode," and has the following options:

* **Propagate:** propagates objects from ancestors to descendants and deletes
  obsolete descendants.
* **Remove:** deletes all existing propagated copies, but does not touch source
  objects.
* **Ignore:** stops modifying this resource. New or changed objects will not be
  propagated, and obsolete objects will not be deleted. The
  `hnc.x-k8s.io/inherited-from` label is not removed. Any unknown mode is
  treated as `Ignore`. This is the default if a resource is not listed at all in
  the config, except for RBAC roles and role bindings (see below).

HNC enforces `roles` and `rolebindings` RBAC resources to have `Propagate` mode.
Thus they are omitted in the `HNCConfiguration` spec and only show up in the
status. You can also set any Kubernetes resource to any of the propagation modes
discussed above. To do so, you need permission to update the `HNCConfiguration`
object.

You can view the current set of resources being propagated, along with
statistics, by saying `kubectl hns config describe`, or alternatively `kubectl
get -oyaml hncconfiguration config`. This object is automatically created for
you when HNC is first installed.

To configure an object resource using the kubectl plugin:

```
# "--group" can be omitted if the resource is a core K8s resource
kubectl hns config set-resource [resource] --group [group] --mode [Propagate|Remove|Ignore]
```

For example:

```
kubectl hns config set-resource secrets --mode Propagate
```

To verify that this worked:

```
kubectl hns config describe

# Output:
Synchronized types:
* Propagating: roles (rbac.authorization.k8s.io/v1)
* Propagating: rolebindings (rbac.authorization.k8s.io/v1)
* Propagating: secrets (v1) # <<<< This should be added
```

You can also modify the config directly to include custom configurations via
`kubectl edit hncconfiguration config`:

```yaml
apiVersion: hnc.x-k8s.io/v1alpha2
kind: HNCConfiguration
metadata:
  name: config
spec:
  resources:
    # Spec for other resources
    ...
    - resource: secrets   <<< This should be added
      mode: Propagate     <<<
```

Adding a new resource in the `Propagate` mode is potentially dangerous, since
there could be existing objects of that resource type that would be overwritten
by objects of the same name from ancestor namespaces. As a result, the HNS
plugin will not allow you to add a new resource directly in the `Propagate`
mode.  Instead, to do so safely:

* Add the new resource in the `Remove` mode. This will remove any propagated
  copies (of which there should be none) but will force HNC to start
  synchronizing all known source objects.
* Wait until `kubectl hns config describe` looks like it's identified the
  correct number of objects of the newly added resource in its status.
* Change the propagation mode from `Remove` to `Propagate`. HNC will then check
  to see if any objects will be overwritten, and will not allow you to change
  the propagation mode until all such conflicts are resolved.

Alternatively, if you're certain you want to start propagating objects
immediately, you can use the `--force` flag with `kubectl hns config
set-resource` to add a resource directly in the `Propagate` mode. You can also
edit the `config` object directly, which will bypass this protection.

<a name="admin-metrics"/>

### Gather metrics

HNC makes the following metrics available, and can be monitored via Stackdriver
(next section) or Prometheus (experimental - see
[#433](https://github.com/kubernetes-sigs/multi-tenancy/issues/433)).

Our [best practices guide](best-practices.md#health) can help you use these
metrics to ensure that HNC stays healthy.

|Metric                                                |Description   |
|:---------------------------------------------------- |:-------------|
| `hnc/namespace_conditions`                           | The number of namespaces affected by [conditions](concepts.md#admin-conditions), tagged with information about the condition |
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
1. Save your some key information as environment variables. You may adjust these
   values to suit your needs; there's nothing magical about them.
   ```bash
   GSA_NAME=hnc-metric-writer
   PROJECT_ID=my-gcp-project

   ```
1. Enable Workload Identity (WI) on either a
   [new](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#enable_workload_identity_on_a_new_cluster)
   or
   [existing](https://cloud.google.com/kubernetes-engine/docs/how-to/workload-identity#enable_workload_identity_on_an_existing_cluster)
   cluster.
1. Install HNC as described [above](#admin-install).
1. [Create a Google service account (GSA)](https://cloud.google.com/docs/authentication/production#creating_a_service_account):
    ```bash
    gcloud iam service-accounts create ${GSA_NAME}
    ```
1. Grant “[Monitoring Metric Writer](https://cloud.google.com/monitoring/access-control#mon_roles_desc)”
role to the GSA:
    ```bash
    gcloud projects add-iam-policy-binding ${PROJECT_ID} --member \
      "serviceAccount:${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com" \
      --role "roles/monitoring.metricWriter"
    ```
1. Create an [Cloud IAM policy binding](https://cloud.google.com/sdk/gcloud/reference/iam/service-accounts/add-iam-policy-binding)
between `hnc-system/default` KSA and the newly created GSA:
     ```
     gcloud iam service-accounts add-iam-policy-binding \
       --role roles/iam.workloadIdentityUser \
       --member "serviceAccount:${PROJECT_ID}.svc.id.goog[hnc-system/default]" \
       ${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com
   ```
1. Add the `iam.gke.io/gcp-service-account=${GSA_NAME}@${PROJECT_ID}` annotation to
the KSA, using the email address of the Google service account:
     ```
     kubectl annotate serviceaccount \
       --namespace hnc-system \
       default \
       iam.gke.io/gcp-service-account=${GSA_NAME}@${PROJECT_ID}.iam.gserviceaccount.com
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

<a name="admin-cli-args">

## Modify command-line arguments

HNC's default manifest file (available as part of each release with the name
`hnc-manager.yaml`) includes a set of reasonable default command-line arguments
for HNC, but you can tweak certain parameters to modify how HNC behaves. These
parameters are different from those controlled by `HNCConfiguration` - they
should only be modified extremely rarely, and only with significant caution.

Interesting parameters include:

* `--apiserver-qps-throttle=&lt;integer&gt;`: set to 50 by default, this limits how many
  requests HNC will send to the Kubernetes apiserver per second in the steady
  state (it may briefly allow up to 50% more than this number). Setting this
  value too high can overwhelm the apiserver and prevent it from serving
  requests from other clients. HNC can easily generate a huge number of
  requests, especially when it's first starting up, as it tries to sync every
  namespace and every propagated object type on your cluster.
* `--enable-internal-cert-management`: present by default. This option uses the
  [ODA `cert-controller`
  library](https://github.com/open-policy-agent/cert-controller) to create and
  distribute the HTTPS certificates used by HNC's webhooks. If you remove this
  parameter, you can replace it with external cert management, such as
  [Jetstack's `cert-manager`](https://github.com/jetstack/cert-manager), which
  must be separately deployed, configured and maintained.
* `--suppress-object-tags`: present by default. If removed, many more tags are
  included in the metrics produced by HNC, including the names of personally
  identifiable information (PII) such as the names of the resource types. This
  can give you more insight into HNC's behaviour at the cost of an increased
  load on your metrics database (through increased metric cardinality) and also
  by increasing how carefully you need to guard your metrics against
  unauthorized viewers.
* `--unpropagated-annotation=&lt;string&gt;`: empty by default, this argument
  can be specified multiple times, with each parameter representing an
  annotation name, such as `example.com/foo`. When HNC propagates objects from
  ancestor to descendant namespaces, it will strip these annotations out of the
  metadata of the _copy_ of the object, if it exists. For example, this can be
  used to remove an annotation on the source object that's has a special meaning
  to another system, such as GKE Config Sync. If you restart HNC after changing
  this arg, all _existing_ propagated objects will also be updated.
