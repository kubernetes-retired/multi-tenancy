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

You can read more about hierarchical namespaces in the [HNC User
Guide](docs/user-guide).

The best way you can help contribute to bringing hierarchical namespaces to the
Kubernetes ecosystem is to try out HNC and report the problems you have with
either HNC itself or its documentation (see below). Or, if it's working well for
you, let us know on the \#wg-multitenancy channel on Slack, or join a
wg-multitenancy meeting. We'd love to hear from you!

With that said, please be cautious - HNC is alpha software. While we will not
break any _existing_ API without incrementing the API version, there may be bugs
or missing features. HNC also requires very high privileges on your cluster and
you should be cautious about installing it on clusters with configurations that
you cannot afford to lose (e.g. that are not stored in a Git repository).

Lead developer: @adrianludwin (aludwin@google.com).

## Using HNC

### Getting started and learning more

The [latest version of HNC is
v0.5.2](https://github.com/kubernetes-sigs/multi-tenancy/releases/tag/hnc-v0.5.2).
To install HNC on your cluster, and the `kubectl-hns` plugin on your
workstation, follow the instructions on that page.

Once HNC is installed, you can try out the [HNC
demos](https://docs.google.com/document/d/1tKQgtMSf0wfT3NOGQx9ExUQ-B8UkkdVZB6m4o3Zqn64)
to get an idea of what HNC can do. Or, feel free to dive right inot the [user
guide](docs/user-guide) instead.

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
* [v0.4 - COMPLETE JUN 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/11):
  stabilize the API and add productionization features.
* [v0.5 - COMPLETE JUL 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/13):
  feature simplification and improved testing and stability.
* [v0.6 - TARGET SEP 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/14):
  introduce the v1alpha2 API and fully automated end-to-end testing.
* [Backlog](https://github.com/kubernetes-sigs/multi-tenancy/milestone/9):
  all unscheduled work.

Non-coding tasks are also tracked in the [HNC
project](https://github.com/kubernetes-sigs/multi-tenancy/projects/4).

## Developing HNC

HNC is a small project, and we have limited abilities to help onboard
developers. If you'd like to contribute to the core of HNC, it would be helpful
if you've created your own controllers before using
[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) and
have a good understanding at least one non-functional task such as monitoring or
lifecycle management. However, there are sometimes tasks to help improve the
CLI or other aspects of usability that require less background knowledge.

With that said, if you want to *use* HNC yourself and are also a developer, we
want to know what does and does not work for you, and we'd welcome any PRs that
might solve your problems.

The main design doc is [here](http://bit.ly/k8s-hnc-design); other design docs
are listed [here](docs/links.md).

### Prerequisites

Make sure you have installed the following libraries/packages and that they're
accessible from your `PATH`:

  - Docker
  - [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
  - [kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/docs/INSTALL.md)
  - [kubebuilder](https://kubebuilder.io)
    - This [Github issue](https://github.com/kubernetes-sigs/controller-runtime/issues/90#issuecomment-494878527)
      may help you resolve errors you get when running the tests or any other
      command.

If you're using `gcloud` and the GCP Container Registry, make sure that `gcloud`
is configured to use the project containing the registry you want to use, and
that you've previously run `gcloud auth configure-docker` so that Docker can use
your GCP credentials.

### Building and deploying to a test cluster

To build from source and deploy to a cluster:
  - Ensure your `kubeconfig` is configured to point at your cluster
    - For example, if you're using GKE, run `gcloud container clusters
      get-credentials <cluster-name> --zone <cluster-zone>`
    - To deploy to KIND, see below instead.
  - Use `make deploy` to deploy to your cluster.
    - Ensure you run `gcloud auth configure-docker` so that `docker-push` works
      correctly.
    - This will also install the `kubectl-hns` plugin into `$GOPATH/bin`. Ensure
      that this is in your `PATH` env var if you want to use it by saying `kubectl
      hns`, as described in the user guide.
    - The manifests that get deployed will be output to
      `/manifests/hnc-controller.yaml` if you want to check them out.
    - Note that `make deploy` can respond to env vars in your environment; see
      the Makefile for more information.
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
* Run `make kind-reset` to stop any existing KIND cluster and setup a new one.
  You don't need to run this every time, only when you're first starting
  development or you think your KIND cluster is in a bad state.
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
* `/hack`: various release scripts, end-to-end bash script tests and other
  miscellaneous files.
* `/internal/reconcilers`: the reconcilers and their tests
* `/internal/validators`: validating admission controllers
* `/internal/forest`: the in-memory data structure, shared between the reconcilers
  and validators
* `/internal/kubectl`: implementation of the `kubectl-hns` plugin

Within the `reconcilers` directory, there are four reconcilers:

* **HNCConfiguration reconciler:** manages the HNCConfiguration via the
  cluster-wide `config` singleton.
* **Anchor reconciler:** manages the subnamespace anchors via
  the `subnamespaceanchor` resources.
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
