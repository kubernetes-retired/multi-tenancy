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

## Using HNC

To install HNC:

```bash
# Install on your cluster
HNC_VERSION=0.2.0-rc1
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml

# Download kubectl plugin (Linux only) - will move to Krew soon
PLUGIN_DIR=<directory where you keep your plugins - just has to be on your PATH>
curl -L https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/kubectl-hierarchical_namespaces -o ${PLUGIN_DIR}/kubectl-hierarchical_namespaces
# If desired to make 'kubectl hns' work:
ln -s ${PLUGIN_DIR}/kubectl-hierarchical_namespaces ${PLUGIN_DIR}/kubectl-hns
```

As a quick start, I'd recommend following the [demo
script](https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64)
mentioned above. For a more in-depth understanding, check out the [HNC Concepts
doc](http://bit.ly/38YYhE0).

## Developing HNC

### Prerequisites

Make sure you have installed the following libraries/packages and that they're
accessible from your `PATH`:
  - Docker
  - [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
  - [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)
  - [kubebuilder](https://kubebuilder.io) - this [Github issue](https://github.com/kubernetes-sigs/controller-runtime/issues/90#issuecomment-494878527) may help you resolve errors you get when running the tests or any other command.

### Deploying to a cluster

To deploy to a cluster:
  - Ensure your `kubeconfig` is configured to point at your cluster
    - For example, if you're using GKE, run `gcloud container clusters
      get-credentials <cluster-name> --zone <cluster-zone>`
    - To deploy to KIND, see below instead.
  - If you're using GCR, make sure that gcloud is set to the project containing
    the GCR repo you want to use, then say `make deploy` to deploy to your cluster.
    - Ensure you run `gcloud auth configure-docker` so that `docker-push` works
      correctly.
    - If you get an error referencing the certificate manager, wait a minute or
      so and then try again. The certificate manager takes a few moments for its
      webhook to become available. This should only happen the first time you
      deploy this way, or if we change the recommended version of cert-manager.
    - This will also install the `kubectl-hierarchical_namespaces` plugin into
      `$GOPATH/bin` (as well as its alias, `kubectl-hns`), so make sure that's
      in your path if you want to use commands like `kubectl hns tree`.
    - The manifests that get deployed will be output to
      `/manifests/hnc-controller.yaml` if you want to check them out.
  - To view logs, say `make deploy-watch`

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

## Releasing

1. Ensure you have a Personal Access Token from Github with permissions to write
   to the repo, and
   [permisison]((https://github.com/kubernetes/k8s.io/blob/master/groups/groups.yaml#L566))
   to access `k8s-staging-multitenancy`'s GCR.

2. Set the following environment variables:

```
export HNC_USER=<your github name>
export HNC_PAT=<your personal access token>
export HNC_IMG_TAG=<the desired image tag, eg v0.1.0-rc1>
```

3. Create a draft release in Github. Ensure that the Github tag name is
   `hnc-$HNC_IMG_TAG`. Save the release in order to tag the repo, or manually
   create the tag beforehand and just save the release as a draft.

4. Get the release ID by calling `curl -u "$HNC_USER:$HNC_PAT"
   https://api.github.com/repos/kubernetes-sigs/multi-tenancy/releases`, finding
   your release, and noting it's ID. Save this as an env var: `export
   HNC_RELEASE_ID=<id>`

5. Call `make release`. Note that your personal access token will be visible in
   the build logs, but will not be printed to the console from `make` itself.
   TODO: fix this (see
   https://cloud.google.com/cloud-build/docs/securing-builds/use-encrypted-secrets-credentials#example_build_request_using_an_encrypted_variable).

6. Exit your shell so your personal access token isn't lying around.

7. Publish the release if you didn't do it already.

8. Test!

After the release, you can run `curl -u "$HNC_USER:$HNC_PAT"
https://api.github.com/repos/kubernetes-sigs/multi-tenancy/releases` to see how
many times the assets have been downloaded.
