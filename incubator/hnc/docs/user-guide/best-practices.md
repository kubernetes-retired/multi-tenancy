# HNC: Best practices and gotchas
_Part of the [HNC User Guide](README.md)_

## Table of contents

* [Gitops integration](#gitops)
* [Eventual consistency and small changes](#consistency)

<a name="gitops"/>

## Gitops integration

Since HNC is controlled by regular Kubernetes objects, you can check your YAML
files into source control and apply them to your cluster(s) via `kubectl apply
-f`. However, HNC does impose two restrictions on the order in which changes can
be applied:

1. A namespace must exist before it can be referenced as the parent of another
   namespace. However, the would-be parent’s `HierarchicalConfiguration` does
   _not_ need to exist.
1. You may not create any cycles between namespaces. For example, assume that
   Namespace A is the parent of B, and you wish to reverse this relationship so
   that B becomes the parent of A. If A’s config is applied before B’s, this
   will result in a cycle.

In both cases, HNC’s validating admission controllers will reject the change.
However, in many cases, simply re-running the `apply` operation will resolve the
issue:

1. If all namespaces are created during the first application, the second
   application will successfully allow them to be referenced as parents.
1. All configs that do _not_ cause a cycle will be applied successfully - in the
   example above, the config for B will be applied successfully, meaning that A
   is no longer the parent of B. Therefore, when A’s config is reapplied, there
   will not be a cycle and the application will succeed.

HNC does not require that the entire cluster’s hierarchy is stored in a single
repo. For example, the top-level roots might be stored in one repo, the
namespaces for the teams _using_ that cluster in another, and each team might
have their own repo as well. Typically, each of these repos will have their own
syncing process.

In such cases, we recommend that each syncing process only be given permissions
in RBAC to modify their relevant subtree. In the example given above, the syncer
for the top-level roots may have cluster-level namespace creation privileges,
and the syncer for the list of teams may only be allowed to create subnamespaces
within the appropriate root. The syncers for each individual teams should only
be allowed to operate within their teams’ subtrees.

<a name="consistency"/>

## Eventual consistency and small changes

As with all Kubernetes controllers, HNC enforces _eventual_ consistency. It does
not attempt to ensure that all properties or objects under its control are
synced in any particular order. For example:

* If A has been successfully set as a parent of B and all the namespace labels
  have been applied, this does _not_ imply that all objects from A have been
  propagated to B.
* Conversely, if the namespace labels suggest that A is the parent of B, B may
  contain objects from other parents. This can either be because B was recently
  the child of another parent, or because B is _becoming_ the child of another
  parent and namespace labels simply haven’t been updated yet.
* If namespace B is changing its parent from A to C, and the same object exists
  in both A and C, there is no guarantee that this object will continue to exist
  in B during the transition.
* If two related objects need to be updated, such as a Role and Role Binding,
  there is no guarantee that they are synced in the order that you expect. For
  example, namespace A might contain a Role named “admins” with very high
  privileges that are restricted to a very small number of people, while
  namespace C might _also_ contain a role named “admins” with more restricted
  but widely shared privileges. While changing the parent from A to C, this
  might result in a large number of people temporarily gaining access to the
  very high privileges until the Role Binding is also updated.
* When namespaces are just being created, or if the controller is restarted,
  many namespaces may have transient critical or non-critical conditions that
  resolve themselves.

As a result, if such intermediate states are a concern, we strongly recommend
making relatively small and easy-to-understand changes to your hierarchy,
whether manually or via Gitops or through other automated systems. Note that
this is a good practice for _any_ kind of deployment, with or without HNC.

When making small changes, ensure that you check for conditions on the affected
namespaces before proceeding to make further changes; `kubectl hns tree <ns>`
will show a summary of all conditions within a subtree.

