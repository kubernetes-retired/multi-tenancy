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

## Getting started

### Prerequisites

Make sure you have installed the following libraries/packages and that they're
accessible from your `PATH`:
  - Docker
  - [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
  - [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)
  - [kubebuilder](https://github.com/kubernetes-sigs/controller-runtime/issues/90#issuecomment-494878527) (_Github issue_). You will most likely encounter this issue when running the tests or any other command.

### Deploying to a cluster

To deploy to a cluster:
  - Ensure your `kubeconfig` is configured to point at your cluster
    - For example, if you're using GKE, run `gcloud container clusters
      get-credentials <cluster-name> --zone <cluster-zone>`
    - To deploy to KIND, see below instead.
  - `IMG=<img> make deploy` to deploy to your cluster. `<img>` must be in a
    container registry that you have permission to access.
    - If you're using GKE, you might want to pick
      `IMG=gcr.io/<project-id>/controller:latest`. Also, ensure you run `gcloud
      auth configure-docker` so that `docker-push` works correctly.
    - If you get an error referencing the certificate manager, wait a minute or
      so and then try again. The certificate manager takes a few moments for its
      webhook to become available. This should only happen the first time you
      deploy this way, or if we change the recommended version of cert-manager.
    - This will also install the `kubectl-hnc` plugin into `$GOPATH/bin`, so
      make sure that's in your path if you want to use commands like `kubectl
      hnc tree`.
    - The manifests that get deployed will be output to
      `/manifests/hnc-controller.yaml` if you want to check them out.
  - To view logs, say `make deploy-watch`

### Using the HNC

To learn how HNC works, I'd recommend following the [demo
script](https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64)
mentioned above.

At this point, you should be able to run the demo script yourself. Please
contact us on Slack if you're having trouble.

### Developing with KIND

While developing the HNC, it's usually faster to deploy locally to
[KIND](https://kind.sigs.k8s.io):

* Run `. devenv` (or `source devenv`) to setup your `KUBECONFIG` env var to
  point to the local Kind cluster.
* Run `make kind-reset` to stop any existing KIND cluster and setup a new one,
  including the cert manager required to run the webhooks. You don't need to run
  this every time, only when you're first starting development or you think your
  KIND cluster is in a bad state.
* Run `CONFIG=kind make deploy` or `make kind-deploy` to build the image, load
  it into KIND, and deploy to KIND. See the notes above for caveats on `make
  deploy`, though you don't need to set `IMG` yourself.
* Alternatively, you can also run the controller locally (ie, not on the
  cluster) by saying `make run`. Webhooks don't work in this mode because I
  haven't bothered to find an easy way to make them work yet.


KIND doesn't integrate with any identity providers - that is, you can't add
"sara@foo.com" as a "regular user." So you'll have to use service accounts and
impersonate them to test things like RBAC rules. Use `kubectl --as
system:serviceaccount:<namespace>:<sa-name>` to impersonate a service account
from the command line, [as documented
here](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#referring-to-subjects).

## Development/code

The directory structure is fairly standard for a Kubernetes project. The most
interesting directories are probably:

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

## Issues and project management

All HNC issues are assigned to an HNC milestone. So far, the following
milestones are defined:

* [v0.1](https://github.com/kubernetes-sigs/multi-tenancy/milestone/7): an
  initial release with all basic functionality so you can play with it, but not
  suitable for any real workloads.
* [v0.2](https://github.com/kubernetes-sigs/multi-tenancy/milestone/8): contains
  enough functionality to be suitable for non-production workloads.
* [Backlog](https://github.com/kubernetes-sigs/multi-tenancy/milestone/9): all
  unscheduled work.

Non-coding tasks are also tracked in the [HNC
project](https://github.com/kubernetes-sigs/multi-tenancy/projects/4).
