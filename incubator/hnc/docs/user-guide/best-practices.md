# HNC: Best practices and gotchas
_Part of the [HNC User Guide](README.md)_

## Table of contents

* [Keeping HNC healthy](#health)
* [Gitops integration](#gitops)
* [Eventual consistency and small changes](#consistency)
* [Getting help](#help)

<a name="health"/>

## Keeping HNC healthy

HNC is installed with very powerful RBAC permissions, and you may rely on it to
enforece your most critical policies. Therefore, it is important to be able to
verify that HNC is running correctly.

### Ensure HNC is running

HNC runs in the `hnc-system` directory by default, as part of the Kubernetes
deployment `hnc-controller-manager`. You should set up monitoring and alerting
to let you know if HNC ever stops running, starts crash-looping, etc.

Potential problems you may encounter include:
* By default, HNC is configured to use up to 100Mi of RAM and 0.1 CPU. This
  should be more than adequate in most cases, but if you observe OOM errors, you
  may want to manually update the manifests or the pod template to increase
  these limits.
* HNC is compatible with reasonable Pod Security Policies (PSPs) - for example,
  it runs as a non-root user without any elevated privileges. If PSPs are
  enabled on your cluster, ensure that the HNC service account
  (`hnc-system/default`) is authorized to use the appropriate policy.

### Logging, monitoring and alerting

HNC provides detailed structured logs on stdout. Please use any logging solution
to access them (e.g. Stackdriver Logging on GKE).

HNC provides multiple [metrics](how-to.md#admin-metrics) that can be monitored
by Stackdriver or (experimentally) Prometheus out of the box. The most important
of these is `hnc/namespace_conditions`, which lists the number of namespaces
with [conditions](concepts.md#admin-conditions). Typically, this number should
be zero, except for brief periods when HNC starts up or when there are rapid
changes being made to the hierarchy.

This metric is tagged both according to the condition code (eg strings such as
`CannotPropagate` or `CritCycle`), as well as whether or not the code is a
critical condition. A good default alerting policy is probably to raise an alert
if there are any conditions that persist for more than about 60s.

### Investigating namespace conditions

The `HNCConfiguration` object with the name `config` is a cluster-wide
singleton, use primarily to configure HNC. However, its
`.status.namespaceConditions` field also gives a summary of all conditions
across the cluster, along with every namespace affected by that condition. You
can view this object via:

```bash
kubectl get hncconfiguration config -oyaml
```

Each namespace will contain more information about its own conditions. For
example, to view detailed information about the conditions in namespace `foo`,
use the following command:

```bash
kubectl hns describe foo
```

<a name="gitops"/>

## Gitops integration

Since HNC is controlled by regular Kubernetes objects, you can check your YAML
files into source control and apply them to your cluster(s) via `kubectl apply
-f`. You generally will only want to do this for your cluster-wide configuration
(the `HNCConfiguration` object) and your full namespace configurations (the
`HierarchyConfiguration` objects in each namespace), and not your subnamespaces
(the `SubnamespaceAnchor` objects) since subnamespaces are mainly for
unprivileged users, not Gitops flows.

When applying your `HierarchyConfiguration` objects, HNC imposes two
restrictions on the order in which changes can be applied:

1. A namespace must exist before it can be referenced as the parent of another
   namespace.
1. You may not create any cycles between namespaces. For example, assume that
   Namespace A is the parent of B, and you wish to reverse this relationship so
   that B becomes the parent of A. If A’s config is applied before B’s, this
   will result in a cycle.

If either condition is violated, HNC’s validating admission controllers will
reject the change. However, in Gitops flows, it is possible to transiently
violate both conditions. For example, a namespace may not be fully created
before it is referenced as a parent, or you might change the parents of multiple
namespaces simultaneously, resulting in a transient cycle between them.

Fortunately, in most cases, simply re-running the `apply` operation will resolve
any issues:

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

<a name="help"/>

## Getting help

When all else fails, talk to a human:

- [Talk to us on Slack](https://kubernetes.slack.com/messages/wg-multitenancy)
- [Ping our mailing list](https://groups.google.com/forum/#!forum/kubernetes-wg-multitenancy)

No guarantees, of course, but we'll do our best to help you out!

Thanks for using HNC.
