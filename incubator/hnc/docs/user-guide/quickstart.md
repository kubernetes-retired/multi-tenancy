# HNC: Quickstarts

***[UPDATE MAY 2021]: HNC has graduated to its [own
repo](https://github.com/kubernetes-sigs/hierarchical-namespaces)! Please visit
that repo for the [latest version of this user
guide](https://github.com/kubernetes-sigs/hierarchical-namespaces/tree/master/docs/user-guide/quickstart.md).***

_Part of the [HNC User Guide](README.md)_

This document walks you through some common ways to use hierarchical namespaces.
It assumes you've already installed HNC on your cluster and the `kubectl-hns`
plugin on your workstation - see [here](how-to.md#admin-install) if you haven't
already done this.

## Contributors

If you update this quickstart, please also update the
[tests](../../test/e2e/quickstart_test.go) that ensure this quickstart keeps
working.

Thanks to our early contributors on the Google Docs version of this page:
@adrianludwin, @sophieliu15, @srampal, @yiqigao217 and Serge Hartman. Thanks
also to all contributors since then.

## Table of contents

* [Basic functionality](#basic)
* [Propagating different resources](#resources)
* [Hierarchical network policy](#netpol)
* [Subnamespaces deep-dive](#subns)
* [Keeping objects out of certain namespaces](#exceptions)

<a name="basic"/>

## Basic functionality

_Demonstrates: setting parent-child relationships, subnamespace creation, RBAC
propagation, hierarchy modification._

Imagine you have an org called _acme-org_. We'll create a _root_ namespace to
represent it:

```bash
kubectl create namespace acme-org
```

The root namespace is just a normal namespace that we'll use as the root of our
hierarchy. A K8s cluster may have many roots; the default case is that every
namespace is the root of its own otherwise empty
[subtree](concepts.md#basic-trees), with no parent or children.

We can also create a namespace for a team within that org, and another namespace
for a service owned by that team. The team might want different services in
different namespaces, since [namespaces are a much better security
boundary](concepts.md#why-ns) than, say, service accounts.

```bash
kubectl create namespace team-a
kubectl create namespace service-1
```

By default, there's no relationship between these namespaces. For example, let's
say that we want to make someone Site Reliability Engineer (SRE) for `team-a`,
so we'll create an RBAC Role and a RoleBinding to that role.

  Note: Typically, we'd create a rolebinding a user account, such as maya@example.com.
  However, every distribution of K8s has different authentication systems, and
  some (like [Kind](https://kind.sigs.k8s.io/)) don't have any at all.
  Therefore, this quickstart uses K8s service accounts instead of user accounts,
  but feel free to use any account that works on your cluster as you follow
  along.

```bash
kubectl -n team-a create role team-a-sre --verb=update --resource=deployments
kubectl -n team-a create rolebinding team-a-sres --role team-a-sre --serviceaccount=team-a:default
```

Similarly, we might want to have a super-SRE group across the whole org:

```bash
kubectl -n acme-org create role org-sre --verb=update --resource=deployments
kubectl -n acme-org create rolebinding org-sres --role org-sre --serviceaccount=acme-org:default
```

Obviously, none of this affects `service-1`, since that's a completely
independent namespace, and RBAC only applies at the namespace level:

```bash
kubectl -n service-1 get rolebindings
```

So this is where the HNC comes in. Let's make `acme-org` the parent of `team-a`,
and `team-a` the parent of `service-1`.

```bash
# Make acme-org the parent of team-a
kubectl hns set team-a --parent acme-org

# This won't work, will be rejected since it would cause a cycle. Try it!
kubectl hns set acme-org --parent team-a

# Make team-a the parent of service-1
kubectl hns set service-1 --parent team-a

# Display the hierarchy
kubectl hns tree acme-org
```

The output will be something like:

```
acme-org
└── team-a
     └── service-1
```

Now, if we check `service-1` again, we'll see that the rolebindings from the
ancestor namespaces have been propagated to the child namespace.

```bash
kubectl -n service-1 describe roles
# Output: you should see two roles, one propagated from acme-org and the other
# from team-a.

kubectl -n service-1 get rolebindings
# Output: similarly, you should see two propagated rolebindings.
```

The controller keeps the RBAC objects in sync with the current hierarchy. For
example, let's say that team-b decides to take over the service. Let's create a
new namespace called `team-b` inside acme-org.

This time, instead of creating the namespace ourselves, we'll let the HNC create
it for us. This is useful if you're administering a subtree and don't have
cluster-wide permissions to create namespaces (see the [subnamespace
quickstart](#subns) for more details).

```bash
kubectl hns create team-b -n acme-org
kubectl hns tree acme-org # may take a few moments to reconcile
```

Expected output:

```
acme-org
├── team-a
│   └── service-1
└── [s] team-b

[s] indicates subnamespaces
```

And team-b is a little weird, they call their SRE's "wizards" so we'll set up their roles too:

```bash
kubectl -n team-b create role team-b-wizard --verb=update --resource=deployments
kubectl -n team-b create rolebinding team-b-wizards --role team-b-wizard --serviceaccount=team-b:default
```

Now, if we assign the service to the new team:

```bash
kubectl hns set service-1 --parent team-b
kubectl hns tree acme-org
```

Expected output:

```
acme-org
├── team-a
└── [s] team-b
    └── service-1
```

And we can verify that the roles and rolebindings have been updated as well.

```bash
kubectl -n service-1 get roles
kubectl -n service-1 get rolebindings
```

<a name="resources"/>

## Propagating different resources

_Demonstrates: simple HNC configuration._

HNC doesn't only work for RBAC. Any Kubernetes resource can be configured to be
propagated through the hierarchy, although by default, only RBAC objects are
propagated.

Continuing from the [previous quickstart](#basic), let's say that the workloads
in `service-1` expect a secret called `my-creds` which is different for each
team. Let's create those creds in `team-b`:

```bash
kubectl -n team-b create secret generic my-creds --from-literal=password=iamteamb
```

If you check existing secrets in `service-1` using the command below, you will
find that the secret does not show up in `service-1` because we haven't
configured HNC to propagate secrets in HNCConfiguration.

```bash
kubectl -n service-1 get secrets
```

In order to get this to work, you need to update the HNCConfiguration object,
which is a single cluster-wide configuration for HNC as a whole. To do this,
simply use the config subcommand:

```bash
kubectl hns config set-resource secrets --mode Propagate
```

  Note: As of HNC v0.6+, the supported modes are `Propagate`, `Remove` and
  `Ignore`. More may be added in the future; you can run `kubectl hns config
  set-resource` for the latest documentation.

Now, we should be able to verify that `my-creds` was propagated to `service-1`:

```bash
kubectl -n service-1 get secrets
```

And finally, note that if we move the service back to team-a, the secret disappears because we haven't created it there:

```bash
kubectl hns set service-1 --parent team-a
kubectl hns tree acme-org
kubectl -n service-1 get secrets
```

If you like, you can also view the entire cluster-wide configuration for HNC, as
well as any conditions in the cluster, by saying `kubectl hns config describe`.
You can also look directly at the underlying cluster-wide configuration and
status object:

```bash
kubectl get hncconfiguration config -o yaml
```

Which should show something like the following:

```yaml
apiVersion: hnc.x-k8s.io/v1alpha2
kind: HNCConfiguration
metadata:
  name: config
spec:
  resources:
  - resource: secrets
    mode: Propagate
status:
  resources:
  - group: rbac.authorization.k8s.io/v1
    mode: Propagate
    numPropagatedObjects: 4
    numSourceObjects: 35
    resource: role
    version: v1
  - group: rbac.authorization.k8s.io/v1
    mode: Propagate
    numPropagatedObjects: 4
    numSourceObjects: 77
    resource: rolebinding
    version: v1
  - mode: Propagate
    resource: secrets
    numPropagatedObjects: 2
    numSourceObjects: 1
    version: v1
```

You can also edit this object directly if you prefer, as an alternative to
using the kubectl plugin - the object is created automatically when HNC is
installed on the cluster.


<a name="netpol"/>

## Hierarchical network policy

_Demonstrates: making Network Policies hierarchy-aware using the 'tree' label._

  Note: this quickstart will only work if network policies are enabled on your
  cluster. Some flavours, such as KIND and GKE, do not enable network policies
  by default.

We now demonstrate propagation of Kubernetes network policies across a namespace
hierarchy. Continuing from the [basic quickstart](#basic), let us add a second
service (`service-2`) under `team-a` and confirm the new view of the hierarchy.

```bash
kubectl hns create service-2 -n team-a
kubectl hns tree acme-org
```

Partial output:

```
acme-org
├── team-a
│   ├── service-1
│   └── service-2
└── [s] team-b
```

Let us now create a web service `s2` in namespace `service-2`, and a client pod
client-s1 in namespace `service-1` that can access this web service. From the
shell in the client container, confirm that we can access the `s2` service running
in namespace `service-2`.

```bash
kubectl run s2 -n service-2 --image=nginx --restart=Never --expose --port 8080

# Verify that it's running:
kubectl get service,pod -o wide -n service-2
```

To test that the service is accessible from workloads in different namespaces,
start a client pod in `service-1` and confirm that the service is reachable.

```bash
kubectl run client -n service-1 -it --image=alpine --restart=Never --rm -- sh
```

If you don't see a command prompt, try pressing enter. In the client pod:

```bash
wget -qO- --timeout 2 http://s2.service-2:8080
```

Confirm you see the nginx web page output, then exit the container shell.

Similarly, you can confirm that the `s2` service can be accessed from pods in
the namespaces team-a and `team-b`.

```bash
kubectl run client -n team-a -it --image=alpine --restart=Never --rm -- sh
```
wget as above and rerun again in `team-b`

Now we'll create a default network policy that blocks any ingress from other
namespaces:

```bash
cat << EOF | kubectl apply -f -
kind: NetworkPolicy
apiVersion: networking.k8s.io/v1
metadata:
  name: deny-from-other-namespaces
  namespace: acme-org
spec:
  podSelector:
    matchLabels:
  ingress:
  - from:
    - podSelector: {}
EOF
```

Now let's ensure this policy can be propagated to its descendants.

```bash
kubectl hns config set-resource networkpolicies --group networking.k8s.io --mode Propagate
```

And verify it got propagated:

```bash
kubectl get netpol --all-namespaces | grep deny
```

Expected output:
```
acme-org    deny-from-other-namespaces   <none>         37s
service-1   deny-from-other-namespaces   <none>         0s
service-2   deny-from-other-namespaces   <none>         0s
team-a      deny-from-other-namespaces   <none>         0s
team-b      deny-from-other-namespaces   <none>         0s
```

If network policies have been correctly enabled on your cluster, we'll now see
that we can no longer access `service-2` from a client in `service-1`:

```bash
kubectl run client -n service-1 -it --image=alpine --restart=Never --rm -- sh
wget -qO- --timeout 2 http://s2.service-2:8080
# Observe timeout
```

But in this example, we'd like all service from a specific team to be able to
interact with each other, so we'll create a second network policy as shown below
that will allow all namespaces within team-a to be able to communicate with each
other. We do this by creating a policy that selects namespaces based on the
[tree label](concepts.md#basic-labels) `<root>.tree.hnc.x-k8s.io/depth`
that is automatically added by the HNC.

```bash
cat << EOF | kubectl apply -f -
kind: NetworkPolicy
apiVersion: networking.k8s.io/v1
metadata:
  name: allow-team-a
  namespace: team-a
spec:
  podSelector:
    matchLabels:
  ingress:
  - from:
    - namespaceSelector:
        matchExpressions:
          - key: 'team-a.tree.hnc.x-k8s.io/depth'
            operator: Exists
EOF
```

Verify that this has been propagated to all descendants of `team-a`:

```bash
kubectl get netpol --all-namespaces | grep allow
```

Expected output:

```
service-1   allow-team-a                 <none>         23s
service-2   allow-team-a                 <none>         23s
team-a      allow-team-a                 <none>         23s
```

Now, we can see that we can access the service from other namespaces in `team-a`,
but not outside of it:

```bash
kubectl run client -n service-1 -it --image=alpine --restart=Never --rm -- sh
wget -qO- --timeout 2 http://s2.service-2:8080
# Expected: nginx web page output
```

Try again in `team-b` but observe that it doesn't work
```bash
kubectl run client -n team-b -it --image=alpine --restart=Never --rm -- sh
wget -qO- --timeout 2 http://s2.service-2:8080
# Expected: wget: download timed out
```

This demo illustrated propagation of network policy through a namespace
hierarchy and how appropriate policies could be used for sub-hierarchies to
facilitate various network isolation models. These examples are not meant to be
a complete solution for use of network policies within an HNC based cluster but
just an illustration of what is possible. In practice these will need to be used
in combination with additional RBAC and other policy controls.

Clean up if desired:

```bash
kubectl delete netpol allow-team-a -n team-a
kubectl delete netpol deny-from-other-namespaces -n acme-org
kubectl delete svc s2 -n service-2
kubectl delete pods s2 -n service-2
```

<a name="subns"/>

### Subnamespaces deep dive

_Will demonstrate: Create and delete [subnamespaces](concepts.md#basic-subns)._

Let's continue to use the example of _acme-org_ that we started from the [basic
quickstart](#basic). Imagine you have a team called _team-a_ under the org
called _acme-org._ You are the admin of the namespace _team-a_ and have no
cluster-wide permissions to create namespaces, but you do have a RoleBinding in
`team-a` that gives you permission to create a `SubnamespaceAnchor` object in
that namespace.

Now you would like to create three subnamespaces for services owned by your
team, say `service-1`, `service-2` and `service-3`:

```bash
kubectl hns create service-1 -n team-a
kubectl hns create service-2 -n team-a
kubectl hns create service-3 -n team-a
kubectl hns tree team-a
```

Expected output:

```
team-a
├── [s] service-1
├── [s] service-2
└── [s] service-3

[s] Indicates subnamespace
```

Now if you want to add a `dev` subnamespace under `service-1`, you can:

```bash
kubectl hns create dev -n service-1
kubectl hns tree team-a
```
Output:
```
team-a
├── [s] service-1
│    └── [s] dev
├── [s] service-2
└── [s] service-3
```

As with regular namespaces ("full namespaces," in HNC terms), subnamespaces must
have unique names. For example, you cannot add a second `dev` subnamespace under
`service-2`. The webhook will prevent you from creating that subnamespace:

```bash
kubectl hns create dev -n service-2
# Error: The requested namespace dev already exists. Please use a different name.
```

You can also delete subnamespaces by deleting the anchor in the parent
namespace. For example:

```bash
kubectl delete ns service-3
# doesn't work

kubectl delete subns service-3 -n team-a
# service-3 is deleted
```

Note that `subns` is a short form for `subnamespaceanchor` or
`subnamespaceanchor.hnc.x-k8s.io`.


Now try to delete `service-1` in the same way, but you'll see it doesn't work:

```bash
kubectl delete subns service-1 -n team-a
# forbidden
```

The reason for this is that `service-1` contains its own subnamespace that would
be deleted with it, because deleting a namespace also deletes all the anchors in
that namespace, and therefore all its subnamespaces. This is very dangerous!

Therefore, just like rm -r won't recursively delete a tree of directories
without an -f option, HNC prevents you from deleting any namespace that contains
subnamespaces by default.

HNC will only allow recursive deletion of subnamespaces if those subnamespaces,
or any of their ancestors, have cascading deletion explicitly set. For example,
to delete `service-1`, we first enable cascading deletion and then remove it:

```bash
kubectl hns set service-1 --allowCascadingDeletion

# Short form:
kubectl hns set service-1 -a
```

Now you can (unsafely!) delete `service-1`:

```bash
kubectl delete subns service-1 -n team-a
```

There's an important difference between subnamespaces and regular child
namespace, also known as a full namespace. A subnamespace is created by HNC due
to an anchor being created in the parent; when that anchor is deleted, the
subnamespace is as well. By contrast, a full child namespace is created without
HNC (e.g., by calling `kubectl create ns regular-child`) and cannot be deleted
by HNC.

That is, a subnamespace's lifespan is tied to its parent, while a full
namespace's lifespan is not. If you delete the parent of a _full_ namespace, the
full namespace itself will _not_ be deleted. However, HNC will mark is as being
in the `ActivitiesHalted (ParentMissing)` condition, as you can see by calling
`kubectl hns describe regular-child`.

Let's see this in action. Create another subnamespace `service-4`:

```bash
kubectl hns create service-4 -n team-a
```

And now create a full namespace that's a child of this subnamespace (yes, full
namespaces can be children of subnamespaces!):

```bash
kubectl create ns staging
kubectl hns set staging --parent service-4
kubectl hns tree team-a
```

Expected output:

```
team-a
├── [s] service-2
├── [s] service-3
└── [s] service-4
     └── staging
```

Now, even if you delete `service-4`, the new `staging` namespace will _not_ be
deleted.

```bash
kubectl delete subns service-4 -n team-a
kubectl hns tree team-a
```
Output:
```
team-a
└── [s] service-2
```

The `staging` namespace no longer shows up, because it's no longer a descendant
of `team-a`. However, if you look at it directly, you'll see a warning that it's
in a bad state:

```bash
kubectl hns describe staging
# See the text "ActivitiesHalted (ParentMissing)"
```

The `ActivitiesHalted` condition indicates that HNC is no longer updating the
objects in this namespace. Instead, it's waiting for an admin to come fix the
problems before it resumes creating or deleting objects. Conditions like these
also show up in the HNC logs, and in its [metrics](how-to.md#admin-metrics).

<a name="exceptions"/>

## Keeping objects out of certain namespaces

_Demonstrates: exceptions_

Now let’s say your `acme-org` has a secret that you originally wanted to share with all the teams. We created this `Secret` as follows:

```bash
kubectl -n acme-org create secret generic my-secret --from-literal=password=iamacme
```

You’ll see that `my-secret` is propagated to both `team-a` and `team-b`:

```bash
kubectl -n team-a get secrets
kubectl -n team-b get secrets
```

But now we’ve started running an untrusted service in `team-b`, so we’ve decided not to share that secret with it anymore. We can do this by setting the propagation selectors on the secret:

```bash
kubectl annotate secret my-secret -n acme-org propagate.hnc.x-k8s.io/treeSelect=!team-b
```

Now you’ll see the secret is no longer accessible from `team-b`:

```bash
kubectl -n team-b get secrets
```

If we add any children below `team-b`, the secret won’t be propagated to them, either.

There are several ways to select the namespaces. The `treeSelect` annotation can
take a list (e.g. `!team-b, !team-a`) to exclude namespaces or a single
namespace (e.g. `team-a`) to include. You can also use the `select` annotation
that takes a [standard Kubernetes label
selector](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors),
or the `none`  annotation to turn off propagation completely. See
[here](how-to.md#use-limit-propagation) for details.

Of course, the annotation can also be part of the object when you create it:

```bash
kubectl delete secret my-secret -n acme-org
cat << EOF | k create -f -
apiVersion: v1
kind: Secret
metadata:
  annotations:
    propagate.hnc.x-k8s.io/treeSelect: team-a
  name: my-secret
  namespace: acme-org
... other fields ...
EOF
```
