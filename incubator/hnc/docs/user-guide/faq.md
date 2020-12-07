# HNC: Frequently asked questions
_Part of the [HNC User Guide](README.md)_

Please feel free to suggest additional questions.

## How do I get in touch with the developers?

You can contact us on:

* [Github issues](https://github.com/kubernetes-sigs/multi-tenancy/issues)
* [Google Groups mailing list](https://groups.google.com/forum/#!forum/kubernetes-wg-multitenancy)
* [#wg-multitenancy on Slack](https://kubernetes.slack.com/messages/wg-multitenancy)

Of these, Github and the mailing list will often get you the fastest response
we're not constantly on Slack.

## What are HNC's minimum requirements?

HNC technically requires Kubernetes 1.15 or higher, although we don't test on
every version of Kubernetes. See the release notes for the version you're
downloading for a full list of the K8s distributions on which that release has
been tested.

By default, HNC's service account is given the equivalent of the [admin cluster
role](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles),
and therefore is able to propagate RoleBindings to that role, since (under the
normal rules of RBAC) an account is not allowed to grant rolebindings to
permission it does not have. For example, HNC is not able to propagate
`cluster-admin` rolebindings.

You may modify HNC's own role bindings in the `hnc-system` namespace to grant it
addition (or fewer) permissions if you wish. At a minimum, HNC must be able to
access (create, read, list, watch, update and delete) all of its own CRs as well
as namespaces, roles, and role bindings.

## Is there a limit to how many levels of child namespaces you can have?

No, HNC does not impose any limitation. We've tested about a hundred, though we
certainly don't recommend that - 3-5 levels should probably be more than enough
for any sane use case.

## How does HNC scale?

HNC is deployed as a single pod with in-memory state, so it cannot scale
horizontally. In practice, we have found the the API throttling by the K8s
apiserver is by far the greatest bottleneck on HNC performance, which would not
be improved via horizontal scaling. Almost all validating webhook calls are also
served entirely by in-memory state and as a result should be extremely fast.

We have tested HNC on clusters with about 500 namespaces, both in an almost-flat
configuration (all namespaces share one common root) and in a well-balanced
hierarchy of a few levels. In the steady state, changes to the hierarchy and any
objects within them were propagated ~instantly. The only real limitation is if
the HNC pod needed to restart, which could take up to 200s on these large
clusters; once again, this was driven almost entirely by the apiserver
limitations. You can adjust the `--apiserver-qps-throttle` parameter in the
manifest to increase it from the default of 50qps if your cluster supports
higher values.

## How much memory does HNC need?

As of Dec. 2020, the [HNC performance test](../../scripts/performance/README.md)
shows that 700 namespaces with 10 propagatable objects in each namespace would
use about 200M memory during HNC startup and about 150M afterwards. Thus, we set
a default of 300M memory limits and 150M memory requests for HNC.

To change HNC memory limits and requests, you can update the values in
`config/manager/manager.yaml`, run `make manifests` and reapply the manifest. If
you are using a GKE cluster, you can view the real-time memory usage in the
`Workloads` tab and determine what's the best limits and requests for you.

## Does HNC support high-availability?

HNC is currently deployed as a single in-memory pod and therefore does not
support HA, as mentioned in the previous section. While it is restarting, it
typically does _not_ block changes to objects, but _will_ block changes to
namespaces, hierarchy configurations and subnamespace anchors.

Please contact us if this does not meet your use case.
