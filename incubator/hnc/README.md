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

You can read more about hierarchical namespaces in the [HNC User Guide](http://bit.ly/38YYhE0).

The best way you can help contribute to bringing hierarchical namespaces to the
Kubernetes ecosystem is to try out HNC and report the problems you have with
either HNC itself or its documentation (see below). Or, if it's working well for
you, let us know on the \#wg-multitenancy channel on Slack, or join a
wg-multitenancy meeting. We'd love to hear from you!

With that said, please be cautious - HNC is alpha software. There are no
guarantees of compatibility or feature support until further notice. HNC also
requires very high privileges on your cluster and you should not install it on
clusters with sensitive configurations that you can't afford to lose.

Lead developer: @adrianludwin (aludwin@google.com).

## Using HNC

### Installing or upgrading HNC
```bash
# Set the desired release:
HNC_VERSION=v0.3.0

# The instructions below are all for HNC v0.3.x. For v0.2.x, please use Git
# history to view an earlier version of this README.

# Install prerequisites on your cluster
kubectl apply -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml

# WAIT for the cert-manager deployments to all become healthy. This can take a
# few minutes.

# Install HNC on your cluster. If this fails due to the cert-manager webhook not
# being ready yet, wait for the webhook to become ready, then re-run it.
kubectl apply -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml

# Download kubectl plugin (Linux only) - will move to Krew soon
PLUGIN_DIR=<directory where you keep your plugins - just has to be on your PATH>
curl -L https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/kubectl-hns -o ${PLUGIN_DIR}/kubectl-hns
chmod +x ${PLUGIN_DIR}/kubectl-hns
```

### Getting started and learning more
As a quick start, I'd recommend following the [HNC demo
scripts](https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64)
to get an idea of what HNC can do. For a more in-depth understanding, check out
the [HNC User Guide](http://bit.ly/38YYhE0).

### Viewing metrics
You should be able to view all HNC metrics in your preferred backend:
* [Stackdriver on GKE](doc/metrics/stackdriver-gke.md)
* Prometheus (see [#433](https://github.com/kubernetes-sigs/multi-tenancy/issues/433))

|Metric                                              |Description   |
|:-------------------------------------------------- |:-------------|
| hnc/reconcilers/hierconfig/total                   | The total number of HierarchyConfiguration (HC) reconciliations happened |
| hnc/reconcilers/hierconfig/concurrent_peak         | The peak concurrent HC reconciliations happened in the past 60s, which is also the minimum Stackdriver reporting period and the one we're using |
| hnc/reconcilers/hierconfig/hierconfig_writes_total | The number of HC writes happened during HC reconciliations |
| hnc/reconcilers/hierconfig/namespace_writes_total  | The number of namespace writes happened during HC reconciliations |
| hnc/reconcilers/object/total                       | The total number of object reconciliations happened |
| hnc/reconcilers/object/concurrent_peak             | The peak concurrent object reconciliations happened in the past 60s, which is also the minimum Stackdriver reporting period and the one we're using |

### Uninstalling HNC
**WARNING:** this will also delete all the hierarchical relationships between
your namespaces. Reinstalling HNC will _not_ recreate these relationships. There
is no need to uninstall HNC before upgrading it.

```bash
rm ${PLUGIN_DIR}/kubectl-hns
kubectl delete -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-${HNC_VERSION}/hnc-manager.yaml

# Don't need to delete the cert manager if you plan to reinstall it later.
kubectl delete -f https://github.com/jetstack/cert-manager/releases/download/v0.11.0/cert-manager.yaml
```

## Issues and project management

All HNC issues are assigned to an HNC milestone. So far, the following
milestones are defined:

* [v0.1 - COMPLETE NOV 2019](https://github.com/kubernetes-sigs/multi-tenancy/milestone/7):
  an initial release with all basic functionality so you can play with it, but
  not suitable for any real workloads.
* [v0.2 - COMPLETE DEC 2019](https://github.com/kubernetes-sigs/multi-tenancy/milestone/8):
  contains enough functionality to be suitable for non-production workloads.
* [v0.3 - COMPLETE APR 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/10):
  type configuration and better self-service namespace UX.
* [v0.4 - IN PROGRESS](https://github.com/kubernetes-sigs/multi-tenancy/milestone/11):
  incremental improvements based on feedback.
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
    - This will install the `kubectl-hns` plugin into `$GOPATH/bin`.
    - The manifests that get deployed will be output to
      `/manifests/hnc-controller.yaml` if you want to check them out.
  - To view logs, say `make deploy-watch`

### Development Workflow

Once HNC is installed via `make deploy`, the development cycle looks like the following:
  - Make changes locally and write new unit and integration tests as necessary
  - Ensure `make test` passes
  - Deploy to your cluster with `make deploy`
  - Monitor changes. Some ways you can do that are:
    - Look at logging with `make deploy-watch`
    - Look at the result of the structure of your namespaces with `kubectl-hns tree -A` or `kubectl-hns tree NAMESPACE`
    - See the resultant conditions or labels on namespaces by using `kubectl describe namespace NAMESPACE`

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
* `/pkg/reconcilers`: the reconcilers and their tests
* `/pkg/validators`: validating admission controllers
* `/pkg/forest`: the in-memory data structure, shared between the reconcilers
  and validators.

Within the `reconcilers` directory, there are four reconcilers:

* **HNCConfiguration reconciler:** manages the HNCConfiguration via the
  cluster-wide `config` singleton.
* **HierarchicalNamespace reconciler:** manages the self-service namespaces via
  the `hierarchicalnamespace` resources.
* **HierarchyConfiguration reconciler:** manages the hierarchy and the
  namespaces via the `hierarchy` singleton per namespace.
* **Object reconciler:** propagates (copies and deletes) the relevant objects
  from parents to children. Instantiated once for every supported object GVK
  (group/version/kind) - e.g., `Role`, `Secret`, etc.

### Releasing

1. Ensure you have a Personal Access Token from Github with permissions to write
   to the repo, and
   [permission](https://github.com/kubernetes/k8s.io/blob/master/groups/groups.yaml#L566)
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
