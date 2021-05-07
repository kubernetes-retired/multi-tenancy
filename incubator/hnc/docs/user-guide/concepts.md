# HNC: Concepts

***[UPDATE MAY 2021]: HNC has graduated to its [own
repo](https://github.com/kubernetes-sigs/hierarchical-namespaces)! Please visit
that repo for the [latest version of this user
guide](https://github.com/kubernetes-sigs/hierarchical-namespaces/tree/master/docs/user-guide/concepts.md).***

_Part of the [HNC User Guide](README.md)_

This section aims to give you a full understanding of what hierarchical
namespaces are, and _why_ they behave the way they do.

## Table of contents

* [Motivation](#why)
  * [Why use regular namespaces?](#why-ns)
  * [Why use hierarchical namespaces?](#why-hns)
  * [When _shouldn’t_ I use hierarchical namespaces?](#why-not-hns)
* [Basic concepts](#basic)
  * [Parents, children, trees and forests](#basic-trees)
  * [Full namespaces and subnamespaces](#basic-subns)
  * [Policy inheritance and object propagation](#basic-propagation)
  * [Namespace labels and non-propagated policies](#basic-labels)
  * [Exceptions and propagation control](#basic-exceptions)
* [Administration](#admin)
  * [Hierarchical Configuration](#admin-hc)
  * [Namespaces administrators](#admin-admin)
  * [Conditions](#admin-conditions)
  * [Labels and annotations read by HNC](#admin-labels-read)
  * [Labels and annotations set by HNC](#admin-labels-set)

<a name="why"/>

## Motivation

<a name="why-ns"/>

### Why use regular namespaces?

Before delving too deep into _hierarchical_ namespaces, it’s worth considering
why Kubernetes has _any_ concept of namespaces in the first place.

Firstly and most obviously, namespaces are a way to organize your Kubernetes
objects, and prevent you from having to find unique names for every object in
your cluster of a given Kind. While two objects of _different_ Kinds (for
example, a Service and an Endpoint) may share the same name, no two objects of
the same Kind may share the same name within a namespace. This makes it easy for
Kubernetes users to use short names, such as “frontend” or “database”, without
colliding with other objects on the same cluster.

More subtly but just as importantly, namespaces are the principal unit of
security isolation and identity in the Kubernetes control plane. They enforce
_exclusivity_ - that is, every namespaced object may belong to only one
namespace. And they allow _grouping_, as described above. These exclusive groups
make them a natural target for sets of related resources. For example,
namespaces are the default target for various policies such as Role Bindings and
Network Policies, as well as extensions to Kubernetes, such as Istio policies.

Of course, it is possible to apply policies _within_ namespaces, but this is
often poorly supported by Kubernetes itself, can be error-prone, and could be
confusing both to human users as well as tools in the Kubernetes ecosystem. For
example, RBAC policies can target objects with individual names, but such
policies are hard to maintain.

As another example, a Pod in a namespace can run as _any_ Service Account from
the same namespace, even one with privileges that far exceed what the Pod
requires; administrators cannot restrict this without resorting to advanced
techniques such as validating admission controllers or [OPA
Gatekeeper](https://github.com/open-policy-agent/gatekeeper). As a result,
applying different policies to different Service Accounts is a weak policy,
unless those Service Accounts are also from different namespaces.

In summary, namespaces are useful constructs for organization, security and
workload identity.

<a name="why-hns"/>

### Why use hierarchical namespaces?

Since namespaces are so useful, you may find yourself creating many namespaces
per cluster. However, just as objects would have been hard to manage without
namespaces, you’ll find that you’ll run into many problems managing all those
namespaces:

* You might want many namespaces to have similar policies applied to them, such
  as to allow access by members of the same team. However, since Role Bindings
  operate at the level of individual namespaces, you will be forced to create
  such Role Bindings in each namespace individually, which can be tedious and
  error-prone. The same applies to other policies such as Network Policies and
  Limit Ranges.
* Similarly, you might want to allow some teams to create namespaces themselves
  as isolation units for their own services. However, namespace creation is a
  privileged cluster-level operation, and you typically want to control this
  privilege very closely.
* Finally, you might want to avoid having to find unique names for every
  namespace in the cluster.

The Hierarchical Namespace Controller addresses the first two of these problems
by allowing you to organize your namespaces into _trees_, and allowing you to
apply policies to those trees (or their subtrees), including the ability to
create new namespaces within those trees.

Unfortunately, HNC cannot solve the requirement that every namespace must be
uniquely named, since this is imposed by Kubernetes itself. However, in practice
this is typically not as stringent a requirement as you might fear. Google’s
internal container management system (Borg) has a concept similar to namespaces,
and tens of thousands of teams regularly use it without any serious conflict.

<a name="why-not-hns"/>

### When _shouldn’t_ I use hierarchical namespaces?

Hierarchies are ideal for expressing ownership and applying default policies but
they are unidimensional - a namespace may only have one parent. However, the
real world is multidimensional; you might want to apply one set of policies to a
namespace based on its ownership, but other (orthogonal) policies based on its
environment (for example,  staging vs. prod) or criticality (for example, batch
vs. realtime).

Unlike hierarchies, labels can be used to implement these kinds of flexible
policies, but they have drawbacks of their own:

* Labels in Kubernetes generally have no permissions; if you have the ability to
  edit an object, you can apply whichever labels you like. This makes them
  unsuitable for policy application unless you trust all the possible editors of
  the relevant objects.
* While labels are more flexible than hierarchies, labels are also much easier
  to get wrong. HNC helps to ensure that most namespaces have a parent and hence
  a healthy set of defaults, while labels can be harder to audit and verify.

If hierarchies do not suit your needs, you can use tools such as the [Namespace
Configuration
Operator](https://github.com/redhat-cop/namespace-configuration-operator) (which
is based on labels) or [Anthos Config
Management](https://cloud.google.com/anthos/config-management) (which supports
both labels and hierarchies). You can also use tools such as [OPA
Gatekeeper](https://github.com/open-policy-agent/gatekeeper) to help ensure that
labels are applied properly, or to directly express and apply policies that
cannot be expressed via hierarchies.

<a name="basic">

## Basic concepts

These concepts are useful for anyone using a cluster with hierarchical
namespaces.

<a name="basic-trees">

### Namespaces, trees, and forests

When using HNC, every namespace may have either zero or one **_parent_**. A
namespace with no parents is known as a **_root_** namespace, while all
namespaces with the same parent are known as the **_children_** of that
namespace. Non-root namespaces may have children of their own; there is no hard
limit to the number of levels of hierarchy. The meaning of the parent-child
relationship is discussed further, but can basically be thought of as one of
_ownership_ - for example, a user with RBAC permissions in a parent will
generally have them in the child as well, since the parent owns the child.

The terms **_ancestor_** and **_descendant_** apply pretty much as you'd expect
(such as the parent of a parent, or the child of a child). All namespaces that
are descendants of a root namespace, along with the root itself, are called a
**_tree_**. The set of all namespaces that descend from _any_ namespace, along
with that namespace, is called a **_subtree_**. Namespaces without children are
called **_leaves_**.

Note that a leaf namespace is technically also a subtree, while a namespace that
is both a root and a leaf is technically also a tree. The set of all trees in
the cluster is known as the **_forest_** of namespaces.

HNC includes validating admission controllers that will stop you from creating
relationship **_cycles_** - for example, two namespaces may not be each others’
parents. HNC maintains an in-memory view of all namespaces in the cluster to
make this feasible.

> _Note: There are some rare corner cases that could result in a cycle being
> formed, despite the presences of the validating admission controllers. For
> example, two different users might make namespaces A and B parents of each
> other at exactly the same time; the admission controller would allow this
> (since neither is yet the parent of the other), leading to a cycle.
> Alternatively, an admin might simply accidentally disable the admission
> controllers. In such cases, HNC will put an `ActivitiesHalted`
> [condition](#admin-conditions) on the namespaces until the cycle is resolved._

In the command line, you may set a namespace’s parent using the `kubectl-hns`
plugin as follows: `kubectl hns set <child> --parent <parent>`. You can also
view the subtree rooted at a namespace via the command `kubectl hns tree <ns>`.
More detailed hierarchical information for that namespace is also available via
`kubectl hns describe <ns>`.

<a name="basic-subns">

### Subnamespaces and full namespaces

Any regular namespace in a cluster may have its parent set. That is, you may
create a namespace via `kubectl create namespace foo`, and then set its parent
via `kubectl hns set foo --parent bar`. However, this first step always requires
cluster-level privileges, which may not be widely granted in your cluster.

To solve this, HNC introduces the concept of **_subnamespaces_**, which is a
namespace that is created as a child of another namespace, and whose lifecycle
is bound to that of its parent. Instead of having cluster-level permissions, you
only need some narrow permissions in the parent namespace. Any namespace that
isn’t a subnamespace is referred to as a **_full namespace_**.

Subnamespaces have two significant differences relative to full namespaces:

* Full namespaces can either be root namespaces or children, and can have their
  parents set, changed, or unset at any time. By contrast, subnamespaces are
  created as a child of another namespace, and this parent can never be changed.
* Full namespaces have independent lifecycles. If the parent of a full namespace
  is deleted, this does not delete the child. By contrast, subnamespaces have
  their lifetimes tied to that of their parent: if the parent is deleted, so is
  the subnamespace. HNC includes features to prevent you from accidentally
  delete subnamespaces, or trees of subnamespaces.

In all other respects, subnamespaces and full namespaces are identical. For
example, subnamespaces must have unique names within the cluster. Subnamespaces
can be parents of full namespaces (and vice versa, of course) although this is
probably not a great idea.

You can create a subnamespace from the command line via `kubectl hns create child
-n parent`.

<a name="basic-propagation">

### Policy inheritance and object propagation

Namespaces in HNC **_inherit_** policies from their ancestors. If you are
familiar with object-oriented languages such as C++ or Java, this will seem
quite natural. Similarly, in many companies, you might have policies that apply
to an entire organization, some that apply to only one department and some that
apply only to individual teams; each of these levels inherits all the policies
from the level above it. This is the model that HNC follows.

The primary way that HNC implements this policy inheritance is simply to copy
them. We refer to this process as **_propagation_**. For example, a Role Binding
in parent namespace `team-a` with a name such as `team-members` will be
propagated to all children of `team-a`; that is, all children of `team-a` will
have its own copy of the `team-members` Role Binding. HNC ensures that these
copies will always stay in sync with the original copy, which is known as the
**_source object_**. The copies are known as **_propagated objects_**.

This implies that any child namespaces, such as `service-1`, are not allowed to
have any independent Role Binding of the same name as that of any of its
ancestors; if it does, HNC will overwrite it. We are considering adding the
concept of **_exceptions_** to HNC to allow policies in child namespaces to
override those from parents in limited circumstances; please let us know if this
would be useful to you.

By default, HNC only propagates RBAC Roles and RoleBindings. However, you can
configure HNC to propagate any other Kind of object, including custom resources.
For example, you might want to propagate the following Kinds of builtin objects:

* **Network Policies:** Allows all workloads within a subtree to be restricted
  to the same ingress/egress rules (but [see below](#basic-labels) for other
  ways to use the hierarchy).
* **Limit Ranges:** Prevents any one pod in a subtree from consuming too many
  resources.
* **Resource Quotas:** Prevents any one namespace in a subtree from consuming
  too many resources. However, users with the ability to create subnamespaces
  can effectively work around this limitation by simply creating a new child
  namespace with the same quota, though efforts are underway to restrict this
  behaviour ([see below](#basic-labels)).
* **Secrets:** Allows Secrets, such as credentials, to be shared across multiple
  namespaces. This is useful if credentials owned by a team must be shared
  across different microservices, or different versions of the same
  microservice, each of which is in its own namespace.
* **Config Maps:** Allows Config Maps to be shared across microservice
  namespaces.

When the original object is updated or deleted, all propagated copies are also
updated or deleted as quickly as possible. Similarly, if you change the parent
of a namespace, any objects that no longer exist in the namespace’s ancestry
will be deleted, and any new objects from that ancestry will be added.

Every propagated object in HNC is given the `hnc.x-k8s.io/inherited-from` label.
The value of this label indicates the namespace that contains the original
object. The HNC admission controller will prevent you from adding or removing
this label, but if you manage to add it, HNC will likely promptly delete the
object (believing that the source object has been deleted), while if you manage
to delete it, HNC will simply overwrite the object anyway.

<a name="basic-labels"/>

### Tree labels and non-propagated policies

While the hierarchy is _defined_ in the [`HierarchicalConfiguration`](#admin-hc)
object, it is _reflected_ on the namespaces themselves via HNC-managed labels.

For example, let’s say that namespace `service-1` has a parent `team-a` and a
grandparent `division-x`. In this case, the namespace `service-1` will have the
following three labels applied to it:

* `service-1.tree.hnc.x-k8s.io/depth: 0`
* `team-a.tree.hnc.x-k8s.io/depth: 1`
* `division-x.tree.hnc.x-k8s.io/depth: 2`

Due to their suffixes, these are known as **_tree labels_**.

Tree labels can be used in two ways. Firstly, any policy that uses namespace
label selectors may use them directly - even if those policies are not
themselves propagated. For example, not only can you put a Network Policy in
`team-a` that will apply to any namespace in that subtree but you can also
configure that policy to allow traffic from any namespace in that subtree by
using the operator `team-a.tree.hnc.x-k8s.io/depth exists`.

Secondly, other applications can use these namespaces to inspect the current
state of the hierarchy. For example, the multitenancy working group heard a
proposal for a hierarchical resource quota to limit resource usage within a
subtree, not just a single namespace. Such applications could directly use the
`.spec.parent` in the `HierarchyConfiguration`, but labels are typically easier
to work with. For example, HNC will ensure that tree labels never express a
cycle, so any labels that are present are guaranteed to be usable.

Note that _in general_, you cannot always trust the values of labels for policy
purposes, because anyone who can edit a Kubernetes object can also apply
whichever labels they like. However, HNC will overwrite any changes made to
these labels, so other applications can trust these labels for policy
application.

<a name="basic-exceptions"/>

### Exceptions and propagation control

By default, HNC propagates _all_ objects of a [specified type](how-to.md#admin-resources)
from ancestor namespaces to descendant namespaces. However, sometimes this is
too restrictive, and you need to create ***exceptions*** to certain policies. For example:

* A ResourceQuota was propagated to many children, but one child namespace now
has higher requirements than the rest. Rather than getting rid of the quota in
the parent namespace, or raising the limit for everyone, you can stop the
quota in the parent from being propagated to that _one_ child namespace,
allowing you to replace it with another, more suitable quota.

* A RoleBinding allows any user to create subnamespaces under one namespace, but
we don’t want to allow those users to create additional levels of hierarchy
underneath those subnamespaces. So you can stop the role binding from being
propagated to _any_ child namespace.

Exceptions are defined using [annotations on the objects
themselves](how-to.md#use-limit-propagation).  As a result, anyone who can edit
an object can also control how it is propagated to descendant namespaces.

If you modify an exception - for example, by removing it - this could cause
the object to be propagated to descendants from which it had previously been
excluded. This could cause you to accidentally overwrite objects that were
intended to be exceptions from higher-level policies, like the ResourceQuota
in the example above. To prevent this, if modifying an exception would cause
HNC to overwrite another object, HNC’s admission controllers will prevent you
from modifying the object, and will identify the objects that would have been
overwritten by your actions. You can then rewrite the exception to safely
exclude those objects, or else delete the conflicting objects to allow them to
be replaced.

<a name="admin"/>

## Administration

These concepts are useful for anyone who needs to administer hierarchical namespaces.

<a name="admin-hc"/>

### Hierarchical Configuration

Until now, we’ve avoided mentioning how the hierarchy is actually represented in
the cluster. The answer is that every namespace may contain a single object with
the Kind `HierarchicalConfiguration` and the name `hierarchy`. This object is
used to configure the subtree rooted at the namespace, and also to report any
problems with the hierarchy. It is also the RBAC attachment point (see
“Namespace administrators,” below).

The parent is defined by the `.spec.parent` field. The `kubectl hns set
--parent` command simply edits this field, but you may also edit it directly in
a yaml file and update it via `kubectl apply -f` (or via `kubectl edit`), if you
have sufficient privileges.

The _status_ of the config is more interesting, as it contains the following
useful properties:

* **Children:** A list of all children of this namespace.
* **Conditions:** A list of all problems affecting this namespace (see more below).

The hierarchical configuration can easily be inspected via the `kubectl hns
describe <ns>` command.

<a name="admin-admin"/>

### Namespaces administrators

Any user (or service account) with the ability to create or update the
hierarchical configuration of a namespace is known as an **_administrator_** of
that namespace from HNC's perspective, even if they have no other permissions
within that namespace. There are two ways you might typically become the
administrator of a namespace:

* You have a Cluster Role Binding that gives you the right to update configs
  across the cluster.
* Someone with that permission has granted you the right to update configs in a
  particular namespace via a Role Binding. Since that Role Binding will be
  propagated to all descendants of that namespace, this typically means you will
  also be an administrator of all descendant namespaces.

Even if you create a root namespace (via `kubectl create namespace foo`), you
are not an administrator of it (from HNC’s perspective) unless you also have
update permissions on the hierarchical config. Conversely, being a namespace
admin does not give you the right to, say, delete that namespace. However, as
with all other Role Bindings, if you are an admin of a namespace, you can grant
that privilege in that namespace or any of its descendants to others.

However, being the admin of a namespace does not give you free reign over that
namespace’s config. For example, if you are the admin of a child namespace but
_not_ its parent, you may not change the parent of your namespace, as this would
remove privileges from the admin of the parent (otherwise known as a
**_superadmin_**, which in HNC's context means an admin of any ancestor
namespace). Similarly, even if your namespace is a root, you may not set its
parent to any namespace of which you are not currently an admin, since the
namespace could then inherit sensitive information such as Secrets.

In general, to change the parent of your namespace N from A to B, you must have
the following privileges:

* You must be the admin of the highest namespace that will _no longer_ be an
  ancestor of namespace N after this change, in order to confirm that you are
  happy to lose your privileges in namespace N.
* You must be the admin of namespace B, in order to acknowledge that sensitive
  objects from B may be copied into namespace N.

A cluster administrator will typically have these privileges, and if A and B are
in the same tree, the admin of their common ancestor will be able to make the
change. However, if no single person exists with these privileges, you could ask
the admin of the root of A to make N a root namespace, then manually grant the
admin of B privileges to N, then ask that admin to make N a child of B.

<a name="admin-conditions">

### Conditions and events

As mentioned above, a **_condition_** is some kind of problem affecting a
namespace or cluster. Namespaces without any problems have all conditions
removed.  Generally speaking, HNC's validating admission webhooks should prevent
most conditions from ever occurring, but there some exceptions and corner cases.
Conditions generally require human intervention to resolve, except as described
below.

Namespace conditions are reported as part of the status of the
`HierarchicalConfiguration` object in each namespace and are exposed via the
`hnc/namespace_conditions` metric. Cluster conditions are reported as part of
the status of the `HNCConfiguration` cluster-wide object; cluster conditions can
either be caused by problems with the cluster-wide configuration, and are also
used to summarize the _namespace_ conditions across the cluster.

HNC conditions follow a subset of the [standard Kubernetes condition
schema](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#Condition) with
the following fields:

* **Type:** one of `ActivitiesHalted` or `BadConfiguration`. The former
  indicates that there's a serious problem that prevents normal HNC operations
  (see more details below), the latter informs cluster admins of a bad set of
  configuration.
* **Reason:** a machine-readable code such as `InCycle` or `ParentMissing` that
  explains why the condition is present.
* **Message:** a human-readable message with more information.

Other standard condition fields, such as `LastTransitionTime` and `Status`, are
unused.

Namespaces with an `ActivitiesHalted` condition have the following properties:

* Object propagation is disabled. That is, new objects will not be copied in,
  and obsolete objects will not be removed.
* The hierarchy within the subtree may not be modified, except to resolve the
  condition (eg break a cycle or replace a missing parent).

When the condition is resolved, object propagation resumes.

When the HNC restarts, there can be a short period during which spurious
conditions may appear on namespaces as HNC restores its internal view of the
cluster’s hierarchy. These are harmless and generally resolve themselves within
10-30 seconds for reasonably sized hierarchies. In all other cases, conditions
require human intervention to resolve.

In addition to problems with the namespaces themselves, HNC may encounter
problems propagating (copying) objects out of source namespaces, or copying them
into destination namespace. In such cases, HNC will generate a standard
[`Event`](https://v1-18.docs.kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#event-v1-core)
for that object, with the `.source.component` field set to `hnc.x-k8s.io`. You
can either query such objects directly, or via `kubectl hns describe NAMESPACE`.
The event will include machine-readable and human-readable information about the
problem, and will generally require human intervention to resolve.

<a name="admin-labels-read">

### Labels and annotations read by HNC

You can modify the behaviour of HNC with various labels and annotations on
objects, in addition to using the custom resources it defines.

#### propagate.hnc.x-k8s.io/TYPE (annotation on objects)

These annotations may be added to any namespaced object to define exceptions to
propagation rules. More information to come.

#### hnc.x-k8s.io/managed-by (annotation on namespaces)

This annotation is mainly designed for use by external products such as GKE
Config Sync or Anthos Config Management, not for human users. These products can
set this annotation for two purposes:

1. To ask HNC to respect _their_ understanding of hierarchy
2. To ask HNC not to interfere with their management of the namespaces

Config Sync and ACM both have their own concept of namespace hierarchy, which
predates HNC's (HNC is actually based on these products). Unlike HNC, these
products only instantiate the _leaf_ namespaces on a cluster, with all
higher-level namespace (which they call _abstract namespaces_) only existing on
a filesystem in a Git repo. However, they have adopted HNC's [tree
labels](#basic-labels) to allow the leaf namespaces to be selected by subtree.

Ordinarily, HNC removes any existing tree labels before replacing them with the
labels it believes are correct, but by setting the `managed-by` annotation,
external products such as ACM can suppress this behaviour and tell HNC to both
_trust_ the existing tree labels, and _propagate_ them to any child namespaces
that may later be created.

Since tree labels are used for policy application, it's dangerous to allow users
to change them simply by adding the `managed-by` annotation to the namespace.
Therefore, HNC only allows this annotation to be added to namespaces that are
roots (from HNC's perspective); similarly, it does not allow you to set a parent
namespace if this annotation already exists. The two are mutually exclusive.

We are considering replacing this with the standard
`app.kubernetes.io/managed-by` label in the future.

<a name="excluded-namespace-label">

#### hnc.x-k8s.io/excluded-namespace (label on namespaces)

***Excluded namespaces configuration is only available in HNC v0.8 and later***

This label should be added to namespaces such as `kube-system` and `kube-public`
so that HNC's validating webhook cannot accidentally prevent operations in these
namespaces and block critical cluster operations. See [Excluding namespaces from
HNC](how-to.md#admin-excluded-namespaces) for more information.

<a name="admin-labels-set">

### Labels and annotations set by HNC

HNC annotates and labels objects in several circumstances. Typically, most users
(or admins) will never need to care about these, but occasionally they may cause
some odd changes in behaviour that you need to be aware of.

#### app.kubernetes.io/managed-by (label on objects)

HNC sets this label on any object that it propagates, taking the place of any
value that might have existed on the source object. It never touches this label
on any source object, such as objects created by Helm or Config Sync.

This label has no meaning _to_ HNC; it's only provided as a way for users to
determine that HNC created an object.

See also `hnc.x-k8s.io/inherited-from`.

#### hnc.x-k8s.io/inherited-from (label on objects)

HNC sets this label on any object that it propagates, similar to
`app.kubernetes.io/managed-by`. Unlike `managed-by`, it identifies the namespace
that held the original copy of this object.

HNC will not allow you to set or modify this label. If you manage to set it on
an object that wasn't originally propagated from a source, HNC will assume that
the source object has been deleted and will therefore immediately delete this
object as well.

#### hnc.x-k8s.io/subnamespace-of (annotation on subnamespaces)

This annotation is placed on any namespace that was created via a [subnamespace
anchor](#basic-subns) and is therefore a subnamespace. It points to the parent
of the subnamespace.

HNC considers a namespace to be a subnamespace if:
* An anchor exists in the parent namespace with the same name as the
  subnamespace, _and_
* The subnamespace contains this annotation pointing to the parent namespace.

If an anchor exists but the subnamespace is missing or incorrect, the anchor
will have its `status.state` set to `Conflict`; deleting a conflicted anchor
will not delete the subnamespace. Conversely, a namespace with the
`subnamespace-of` annotation but no anchor in the parent will have a condition
with the `SubnamespaceAnchorMissing` code in its `HierarchyConfiguration`
object; this can be resolved either by removing the annotation or creating the
anchor in the parent namespace.

Generally speaking, you should never have to look at or modify this annotation.
The one exception is if you would like to convert a subnamespace into a full
namespace - which is to say, you no longer want its lifetime to be controlled by
its anchor. In such cases, you can do the following:

1. Remove the `hnc.x-k8s.io/subnamespace-of` annotation from the subnamespace.
2. Ensure that the anchor in the parent is in the `Conflict` state, and then
   delete the anchor.

At this point, the namespace will still be a _child_ of its parent, but you can
now move it around the hierarchy (e.g. via `kubectl hns NS set --parent
NEW_PARENT`). There is no need (and typically no good reason) to take a full
namespace and turn it back into a subnamespace.

#### NAMESPACE.tree.x-k8s.io/depth (label on namespaces)

This is the [tree label](#basic-labels) which is documented above. It is set by
HNC on namespaces and (unless `managed-by` was set first) cannot be modified by
anyone other than HNC.
