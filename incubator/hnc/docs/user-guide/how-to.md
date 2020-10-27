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
  * [Backing up and restoring HNC data](#admin-backup-restore)
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
recommend installing the `kubectl-hns` plugin.

You can install the plugin by following the instructions for the [latest
release](https://github.com/kubernetes-sigs/multi-tenancy/releases/tag/hnc-v0.5.3).

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

$ kubectl get -oyaml -nparent subns child
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

This section is under construction (as of May 2020). TL;DR: `kubectl hns tree <name>` and `kubectl hns describe <name>`.

TODO: explain conditions (eg get HNC to try to propagate a `cluster-admin` rolebinding).

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

**WARNING: this guard against creating ancestor objects was only introduced in
HNC v0.5.3. Earlier versions of HNC have inconsistent behaviour; see #1076 for
details.**

However, if you bypass these admission controllers - for example, by updating
objects while HNC is being upgraded - HNC _will_ overwrite conflicting objects
in descendant namespaces. This is to ensure that if you are able to successfully
create a policy in an ancestor namespace, you can be confident that it will be
uniformly applied to all descendant namespaces.

HNC can also propagate objects other than RBAC objects, but only cluster
administrators can modify this. See [here](#admin-types) for instructions.

Occasionally, objects might fail to be propagated to descendant namespaces for a
variety of reasons - e.g., HNC itself might not have sufficient RBAC
permissions. To understand why an object is not being propagated to a namespace,
use `kubectl hns describe <ns>`, where `<ns>` is either the source (ancestor) or
destination (descendant) namespace.

<a name="use-select"/>

### Select namespaces based on their hierarchies

HNC inserts labels onto your namespaces to allow trees (and subtrees) of
namespaces to be selected by policies such as `NetworkPolicy`.

This section is under construction (as of Apr 2020). For now, please see
[this demo](https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64/edit#heading=h.9j9w6spvzkdn).

<a name="use-subns-delete"/>

### Delete a subnamespace

In order to delete a subnamespace, you must first have permissions to delete its
anchor in its parent namespace. Ask your cluster administrator to give you this
permission if you do not have it.

**WARNING: the protections described in this section only work on clusters with
Kubernetes 1.15 and higher installed. See [issue
#688](https://github.com/kubernetes-sigs/multi-tenancy/issues/688) for details.
In Kubernetes 1.14 and earlier, HNC is unable to stop you from deleting
namespaces.**

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

> **WARNING: this option is very dangerous, so you should only set it on the lowest
possible level of the hierarchy.**

> **WARNING: any subnamespaces of the namespace you are deleting will also be
deleted, and so will any subnamespaces of those namespaces, and so on. However,
any _full_ namespaces that are descendants of a subnamespace will not be
deleted.**

> _Note: In HNC v0.5.x and earlier, HNC uses v1alpha1 API and this field is
> called `allowCascadingDelete`._

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

#### Install an official release

[The most recent official release is
v0.5.3](https://github.com/kubernetes-sigs/multi-tenancy/releases/tag/hnc-v0.5.3).
Please see that page for release notes and installation instructions.

#### Download the kubectl plugin

The `kubectl-hns` plugin makes most HNC use and administration much easier; we
strongly recommend installing it. To install it, please follow the instruction 
on [Prepare to use hierarchical namespaces](#use-prepare).

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

To temporarily disable HNC, simply delete its deployment and webhooks:

```bash
kubectl -n hnc-system delete deployment hnc-controller-manager
kubectl delete validatingwebhookconfiguration.admissionregistration.k8s.io hnc-validating-webhook-configuration
```

You may also completely delete HNC, including its CRDs and namespaces. However,
**this is a destructive process that results in some data loss.** In particular,
you will lose any cluster-wide configuration in your `HNCConfig` object, as well
as any hierarchical relationships between different namespaces, _excluding_
subnamespaces (subnamespace relationships are saved as annotations on the
namespaces themselves, and so can be recreated when HNC is reinstalled).

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
#
# NB: this process is somewhat broken in HNC v0.4.x. Upgrade to v0.5.x
# before attempting this, or else be prepared to look for resources that
# aren't deleted and manually remove their finalizers. See issue #824 for
# more information.
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
would require them to set the `allowCascadingDeletion` property of the child
namespace.

<a name="admin-types"/>

### Modify the object types propagated by HNC

Starting from HNC v0.6, HNC supports the following propagation modes for each
resource:
* `Propagate`: propagates objects from ancestors to descendants and deletes
  obsolete descendants.
* `Remove`: deletes all existing propagated copies.
* `Ignore`: stops modifying this resource. New or changed objects will not be
  propagated, and obsolete objects will not be deleted. The `inherited-from`
  label is not removed. Any unknown mode is treated as `Ignore`.

HNC enforces `roles` and `rolebindings` RBAC resources to have `Propagate` mode.
Thus they are omitted in the `HNCConfiguration` spec and only show up in the
status. You can also set any Kubernetes resource to any of the propagation modes
discussed above. To do so, you need cluster privileges.

Note: Before HNC v0.6, the propagation modes were in lower case (`propagate`,
`remove`, `ignore`). The modes were set on types by `apiVersion` and `kind` instead
of `group` and `resource`. The `Role` and `RoleBinding` RBAC kinds were also
enforced but they were still left in the `HNCConfiguration` spec.

**WARNING: If you start propagating a new object type, HNC _cannot_ check
whether there are conflicting objects in descendant namespaces, and will
overwrite them. This will be fixed in HNC v0.6 (see #1102).**

To configure an object type using the kubectl plugin:

```
# Starting from HNC v0.6:
# "--group" can be omitted if the resource is a core K8s resource
kubectl hns config set-resource [resource] --group [group] --mode [Propagate|Remove|Ignore]

# Before HNC v0.6:
kubectl hns config set-type --apiVersion [apiVersion] --kind [kind] [propagate|remove|ignore]
```

For example:

```
# Starting from HNC v0.6:
# "--group" can be omitted if the resource is a core K8s resource
kubectl hns config set-resource secrets --mode Propagate

# Before HNC v0.6:
kubectl hns config set-type --apiVersion v1 --kind Secret propagate
```

To verify that this worked:

```
kubectl hns config describe
# Output starting from HNC v0.6:
Synchronized types:
* Propagating: roles (rbac.authorization.k8s.io/v1)
* Propagating: rolebindings (rbac.authorization.k8s.io/v1)
* Propagating: secrets (v1) # <<<< This should be added

# Output before HNC v0.6:
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

Modify the config to include custom configurations:

```
# Starting from HNC v0.6:
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
      
# Before HNC v0.6:
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

Our [best practices guide](best-practices.md#health) can help you use these
metrics to ensure that HNC stays healthy.

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
