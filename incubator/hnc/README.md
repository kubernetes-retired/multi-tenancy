# The Hierarchical Namespace Controller (HNC)

```bash
$ kubectl hns create child -n parent
$ kubectl hns tree parent
parent
└── child
```

Hierarchical namespaces make it easier for you to create and manage namespaces
in your cluster. For example, you can create a hierarchical namespace under your
team's namespace, even if you don't have cluster-level permission to create
namespaces, and easily apply policies like RBAC and Network Policies across all
namespaces in your team (e.g. a set of related microservices).

You can read more
about hierarchical namespaces in the [HNC Concepts doc](http://bit.ly/38YYhE0).

The best way you can help contribute to bringing hierarchical namespaces to the
Kubernetes ecosystem is to try out HNC and report the problems you have (see
below). Or, if it's working well for you, let us know on the \#wg-multitenancy
channel on Slack, or join a MTWG meeting. We'd love to hear from you!

With that said, please be cautious - HNC is pre-alpha software. There are no
guarantees of compatibility or feature support until further notice. HNC also
requires very high privileges on your cluster and you should not install it on
clusters with sensitive configurations that you can't afford to lose.

Lead developer: @adrianludwin (aludwin@google.com).

## Using HNC

### Installing or upgrading HNC
```bash
# Set the desired release:
HNC_VERSION=v0.2.0

# Install prerequisites on your cluster
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
# WAIT for the cert-manager deployments to all become healthy. This can take a
# minute or two.

# Install HNC on your cluster. If this fails due to the cert-manager webhook not
# being ready yet, just re-run it.
kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml

# Download kubectl plugin (Linux only) - will move to Krew soon
PLUGIN_DIR=<directory where you keep your plugins - just has to be on your PATH>
curl -L https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/kubectl-hierarchical_namespaces -o ${PLUGIN_DIR}/kubectl-hierarchical_namespaces
chmod +x ${PLUGIN_DIR}/kubectl-hierarchical_namespaces
# If desired to make 'kubectl hns' work:
ln -s ${PLUGIN_DIR}/kubectl-hierarchical_namespaces ${PLUGIN_DIR}/kubectl-hns
```

### Getting started and learning more
As a quick start, I'd recommend following the [HNC demo
scripts](https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64)
to get an idea of what HNC can do. For a more in-depth understanding, check out
the [HNC Concepts doc](http://bit.ly/38YYhE0).

### Uninstalling HNC
**WARNING:** this will also delete all the hierarchical relationships between
your namespaces. Reinstalling HNC will _not_ recreate these relationships. There
is no need to uninstall HNC before upgrading it.

```bash
rm ${PLUGIN_DIR}/kubectl-hns
rm ${PLUGIN_DIR}/kubectl-hierarchical_namespaces
kubectl delete -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml
kubectl delete -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
```

## Issues and project management

All HNC issues are assigned to an HNC milestone. So far, the following
milestones are defined:

* [v0.1 - COMPLETE](https://github.com/kubernetes-sigs/multi-tenancy/milestone/7):
  an initial release with all basic functionality so you can play with it, but
  not suitable for any real workloads.
* [v0.2 - COMPLETE](https://github.com/kubernetes-sigs/multi-tenancy/milestone/8):
  contains enough functionality to be suitable for non-production workloads.
* v0.3: definition in progress (as of Jan 2020)
* [Backlog](https://github.com/kubernetes-sigs/multi-tenancy/milestone/9):
  all unscheduled work.

Non-coding tasks are also tracked in the [HNC
project](https://github.com/kubernetes-sigs/multi-tenancy/projects/4).

## Developing HNC

HNC is a small project, and we have limited abilities to help onboard developers
and review pull requests at this time. However, if you want to *use* HNC
yourself and are also a developer, we want to know what does and does not work
for you, and we'd welcome any PRs that might solve your problems.

* Design doc: http://bit.ly/k8s-hnc-design
* Demonstration: https://youtu.be/XFZhApTlJ88?t=171 (MTWG meeting; Sep 24 '19)
  * Script for said demo: https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64

### Prerequisites

Make sure you have installed the following libraries/packages and that they're
accessible from your `PATH`:
  - Docker
  - [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
  - [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)
  - [kubebuilder](https://kubebuilder.io) - this [Github issue](https://github.com/kubernetes-sigs/controller-runtime/issues/90#issuecomment-494878527) may help you resolve errors you get when running the tests or any other command.

### Building and deploying to a test cluster

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

### Code structure

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
  (group/version/kind) - e.g., `Role`, `Secret`, etc.

### Releasing

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

5. Call `make release`. **Note that your personal access token will be visible
   in the build logs,** but will not be printed to the console from `make`
   itself.  TODO: fix this (see
   https://cloud.google.com/cloud-build/docs/securing-builds/use-encrypted-secrets-credentials#example_build_request_using_an_encrypted_variable).

6. Exit your shell so your personal access token isn't lying around.

7. Publish the release if you didn't do it already.

8. Test! At a minimum, install it onto a cluster and ensure that the controller
   comes up.

9. Update the "installing HNC" section of this README with the latest version.

After the release, you can run `curl -u "$HNC_USER:$HNC_PAT"
https://api.github.com/repos/kubernetes-sigs/multi-tenancy/releases/$HNC_RELEASE_ID`
to see how many times the assets have been downloaded.
