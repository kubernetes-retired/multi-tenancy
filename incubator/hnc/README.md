# The Hierarchical Namespace Controller (HNC)

This is an early concept to allow namespaces to be linked to each other in
parent-child relationships, and to allow certain objects from ancestors to be
"visible" in their descendents, which is achieved by copying them. This is most
useful for objects such as RBAC roles and bindings - a rolebinding made in a
parent namespace will (under normal circumstances) also grant access in child
namespaces as well, allowing for hierarchical administration.

The HNC is probably most useful in multi-team environments - that is, clusters
shared by multiple teams in the same company or organization. It is _not_
suitable for "hard multitenancy" and does _not_ attempt to add any isolation
features to K8s. It can also be used in single-tenant scenarios, such as when
you want to run multiple types of services in their own namespaces, but with
common policies applied to them.

Status: pre-alpha, no guarantees of compatability or feature support until
further notice.

Design doc: http://bit.ly/k8s-hnc-design

Developers: adrianludwin@ (Google). Please contact me if you want to help out,
or just join a MTWG meeting.

## Usage

Install the operator as you would any other kubebuilder controller (eg `make
install`, `make deploy`). No prebuilt images exist yet.

### Labels or singletons?

The design doc proposes two different ways to define the hierarchy:

1. Through a "singleton" object called `hier` that exists in every namespace -
   specifically, via `.spec.parent`.
1. Through well-known labels on the namespaces, `hnc.x-k8s.io/parent`.

The two methods are mutually exclusive; to enable the second, pass the
`--labels-only` flag to the controller when starting it. For now, the second is
actually set as the default, both in `make run` and also in
`/config/manager/manager.yaml`.

Once we make a permanent decision on which UX to offer, we'll delete the other.

## Development/code

Most of the interesting code is in `/controllers`, with a bit in `/pkg` as well.
There are four controllers, all of which are mutually exclusive except the
Object controller:

* **Hierarchy controller:** manages the hierarchy based on the `Hierarchy` singleton
  (only used when `--labels-only` is not set).
* **Label controller:** manages the hierarchy based on the namespace labels (only
  used when `--labels-only` _is_ set).
* **Namespace controller:** Creates the hierarchy singleton when applicable.
* **Object controller:** Propagates (copies and deletes) the relevant objects
  from parents to children. Instantiated once for every supported object GVK
  (group/version/kind) - currently, `Role`, `RoleBinding`, `Secret` and
  `ConfigMap`.

In addition, the in-memory version of the hierarchy forest is located in
`pkg/forest`.

## Open issues

Too many to count, but at a very high level:

* Write webhooks to prevent bad input (eg cycles) and implement RBAC rules
* Add configuration (eg ability to disable Secrets propagation)
* Reliability and scalability are fully untested, ie:
  * Setting the resync period
  * Testing for crashes at various points
  * Parallelizing concurrent operations
* Upgrade paths
* Auditing, events, etc

## Testing

I do most of my testing in [KIND](https://kind.sigs.k8s.io).

### Helpful scripts

There are a bunch of utilities in `/scripts` which are essentially wrapped
`kubectl` commands. For example, `/scripts/ns-set-parent foo bar` will set the
`parent` label on the `foo` namespace to point to the `bar` namespace using the
following tricky-to-remember command:

```
kubectl patch namespace $1 -p"{\"metadata\": {\"labels\": {\"hnc.x-k8s.io/parent\": \"$2\"}}}"
```

_NB: this command only works when --labels-only was set; otherwise, use
`hier-set-parent` instead._

Other commands are typically simpler, apart from those used to control KIND
itself (see below).

### Testing on KIND (Kubernetes IN Docker)

* Run `. ./scripts/kind-env` to setup your `KUBECONFIG` env var correctly.
  * NB: This sets env vars, hence the need to source (and not run) it.
* Run `./scripts/kind-reset` to stop any existing KIND cluster and setup a new
one, including the cert manager required to run the webhooks.
* Run `make test` to run the controller (excluding the validating webhook)
  locally.
* Run `./scripts/kind-deploy` to build the image, load it into KIND, and deploy
  to KIND, then `./scripts/kind-watch` to watch the logs from the HNC container.

KIND doesn't integrate with any identity providers - that is, you can't add
"sara@foo.com" as a "regular user." So you'll have to use service accounts and
impersonate them to test things like RBAC rules. Use `kubectl --as
system:serviceaccount:<namespace>:<sa-name>` to impersonate a service account
from the command line, [as documented
here](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#referring-to-subjects).
