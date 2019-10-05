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

* Design doc: http://bit.ly/k8s-hnc-design
* Demonstration: https://youtu.be/XFZhApTlJ88?t=171 (MTWG meeting; Sep 24 '19)
  * Script for said demo: https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64

Developers: @adrianludwin (Google). Please contact me if you want to help out,
or just join a MTWG meeting.

## Usage

Make sure you have downloaded the following libraries/packages:
  - [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
  - [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)
  - [kubebuilder](https://github.com/kubernetes-sigs/controller-runtime/issues/90#issuecomment-494878527) (_Github issue_). You will most likely encounter this issue when running the tests or any other command.

Install the operator as you would any other kubebuilder controller (eg `make
install`, `make deploy`). No prebuilt images exist yet.

## Development/code

The directory structure is fairly standard for a Kubebuilder v1 controller
(the HNC actually uses Kubebuilder v2, but the default directory structure was
too limiting). The most interesting directories are probably:

* `/api`: the API definition.
* `/cmd`: top-level executables. Currently the manager and the kubectl plugin.
* `/pkg/controllers`: the reconcilers and their tests
* `/pkg/validators`: validating admission controllers
* `/pkg/forest`: the in-memory data structure, shared between the controllers
  and validators.

Within the `controllers` directory, there are two controller:

* **Hierarchy controller:** manages the hierarchy via the `Hierarchy` singleton
  as well as the namespace in which it lives.
* **Object controller:** Propagates (copies and deletes) the relevant objects
  from parents to children. Instantiated once for every supported object GVK
  (group/version/kind) - currently, `Role`, `RoleBinding` and `Secret`.

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

### Testing on KIND (Kubernetes IN Docker)

* Run `. devenv` (or `source devenv`) to setup your `KUBECONFIG` and `PATH` env vars correctly.
* Run `./kind-reset` to stop any existing KIND cluster and setup a new one,
  including the cert manager required to run the webhooks.
* Run `make test` to run the controller (excluding the validating webhook)
  locally.
* Run `./kind-deploy` to build the image, load it into KIND, and deploy to KIND,
  then `./kind-watch` to watch the logs from the HNC container.

KIND doesn't integrate with any identity providers - that is, you can't add
"sara@foo.com" as a "regular user." So you'll have to use service accounts and
impersonate them to test things like RBAC rules. Use `kubectl --as
system:serviceaccount:<namespace>:<sa-name>` to impersonate a service account
from the command line, [as documented
here](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#referring-to-subjects).
