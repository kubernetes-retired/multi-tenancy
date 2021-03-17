# The Hierarchical Namespace Controller (HNC)

```bash
$ kubectl hns create my-service -n my-team
$ kubectl hns tree my-team
my-team
└── my-service
```

Hierarchical namespaces make it easier to share your cluster by making
namespaces more powerful. For example, you can create additional namespaces
under your team's namespace, even if you don't have cluster-level permission to
create namespaces, and easily apply policies like RBAC and Network Policies
across all namespaces in your team (e.g. a set of related microservices).

Learn more in the [HNC User Guide](docs/user-guide) or get started with the
instructions below!

Credits:
* Lead developer: @adrianludwin (aludwin@google.com)
* Current contributors: @yiqigao217, @rjbez17, @GinnyJI
* Other contributors include @sophieliu15, @lafh, @shivi28, @danielSbastos and @entro-pi - thanks all!

## Using HNC

<a name="start"/>

### Getting started and learning more

The [latest version of HNC is
v0.7.0](https://github.com/kubernetes-sigs/multi-tenancy/releases/tag/hnc-v0.7.0).
To install HNC on your cluster, and the `kubectl-hns` plugin on your
workstation, follow the instructions on that page.

HNC is also supported by the following vendors:

* GKE: [install via Config Sync](https://cloud.google.com/kubernetes-engine/docs/add-on/config-sync/how-to/installing-hierarchy-controller)
* Anthos: [install via ACM](https://cloud.google.com/anthos-config-management/docs/how-to/installing-hierarchy-controller)

Once HNC is installed, you can try out the [HNC
quickstart](https://bit.ly/hnc-quickstart)
to get an idea of what HNC can do. Or, feel free to dive right into the [user
guide](docs/user-guide) instead.

### Roadmap and issues

Please file issues - the more the merrier! Bugs will be investigated ASAP, while
feature requests will be prioritized and assigned to a milestone or backlog.

HNC is not yet GA, so please be cautious about using it on clusters with config
objects you can't afford to lose (e.g. that aren't stored in a Git repository).

All HNC issues are assigned to an HNC milestone. So far, the following
milestones are defined or planned:

* v1.0 - likely late Q1 or early Q2 2021: HNC recommended for production use
* v0.8 - likely early 2021, incremental stability improvements
* [v0.7 - COMPLETE DEC 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/18):
  introduce exceptions.
* [v0.6 - COMPLETE OCT 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/14):
  introduce the v1alpha2 API and fully automated end-to-end testing.
* [v0.5 - COMPLETE JUL 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/13):
  feature simplification and improved testing and stability.
* [v0.4 - COMPLETE JUN 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/11):
  stabilize the API and add productionization features.
* [v0.3 - COMPLETE APR 2020](https://github.com/kubernetes-sigs/multi-tenancy/milestone/10):
  type configuration and better self-service namespace UX.
* [v0.2 - COMPLETE DEC 2019](https://github.com/kubernetes-sigs/multi-tenancy/milestone/8):
  contains enough functionality to be suitable for non-production workloads.
* [v0.1 - COMPLETE NOV 2019](https://github.com/kubernetes-sigs/multi-tenancy/milestone/7):
  an initial release with all basic functionality so you can play with it, but
  not suitable for any real workloads.
* [Backlog](https://github.com/kubernetes-sigs/multi-tenancy/milestone/9):
  all unscheduled work.

## Contributing to HNC

The best way you can help contribute to bringing hierarchical namespaces to the
Kubernetes ecosystem is to try out HNC and report the problems you have with
either HNC itself or its documentation. Or, if it's working well for you, let us
know on the \#wg-multitenancy channel on Slack, or join a wg-multitenancy
meeting. We'd love to hear from you!

But if you're looking for a deeper level of involvement, read on...

### Developing HNC

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

If you are developing on MacOS, you will need gnu-sed. Instructions are below:
```
brew install gnu-sed
ln -s $(which gsed) /usr/local/bin/sed # sym link sed to gsed
```

### Development Workflow

Once HNC is installed via `make deploy` (see next sections), the development
cycle looks like the following:

  - Start a new branch and make all your changes there (this is just the
    standard Github flow).
  - Make changes locally and write new unit and e2e tests as necessary
    - Unit tests are located in the same directory as the code, and either test
      the code directly (e.g. in `internal/object`) or use the K8s test env,
      which is basically just an apiserver (e.g. in `internal/reconcilers`).
    - End-to-end (e2e) tests are located in `test/e2e/`, or other more
      special-purpose directories in `test/`. These are basically glorified
      `bash` scripts with better error checking, and directly invoke `kubectl`.
  - Ensure `make test` passes - this runs unit tests only.
    - If you see all tests fail with `no matches for v1/CRD` [error](https://gist.github.com/yiqigao217/9394c2aadaa515e82184684a005187af)
      , remove your `/usr/local/kubebuilder/` directory and [reinstall kubebuilder](https://book.kubebuilder.io/quick-start.html#installation).
  - Deploy to your cluster with `make deploy`
  - Test your changes by hand and verify that your changes are working
    correctly. Some ways you can do that are:
    - Look at logging with `make deploy-watch`
    - Look at the result of the structure of your namespaces with `kubectl-hns tree -A` or `kubectl-hns tree NAMESPACE`
    - See the resultant conditions or labels on namespaces by using `kubectl describe namespace NAMESPACE`
  - Run the e2e tests via `make test-e2e`. This will take about 10-20m, so go
    get a coffee or walk around your house if you're quarantined.
    - Note that the deployment must be ready _before_ you start the tests.
    - You need to set the `HNC_REPAIR` env var to point to the absolute path of
      the manifests used to deploy HNC - either the released versions (e.g.
      stored on Github) or the _full_ path to the local manifests. If these are
      not set, we'll skip any tests that include modifying the HNC deployement,
      e.g. to bypass webhooks.
    - If you deploy to Kind, the tests that use Network Policies may fail. See
      https://github.com/kubernetes-sigs/multi-tenancy/issues/1098.
  - Once you're ready to make a pull request, please follow the following
    instructions:
    - Each PR should contain _one commit_. If you have multiple commits (either
      introduced during your development or as a result of comments during the
      PR review process), please squash them into a single commit. Force-push
      any changes to your fork of this repo.
    - Ensure your commit message includes a "Tested:" section explaining in
      reasonable detail what you did to test your change.
      [Here](https://github.com/kubernetes-sigs/multi-tenancy/commit/ce983662e87264c76f92dbfbab7cef7bd6128837)
      is a good example. A minimal message might be something like "Added new
      test; verified that the test failed before my change and passed after it;
      ran e2e tests."
    - Create the PR. We'll try to reply fairly quickly!
    - Make any requested changes (using `git commit --amend` and `git push -f`
      to ensure you only have one commit).
    - Profit! Or at least, enjoy your feeling of accomplishment.

If you need to make a larger change, please write your plan down somewhere - for
example, either in a Github issue or in a short Google doc
[example](https://docs.google.com/document/d/17J8icBEDvLLoPT4kQ4ArZcCerRweDY-TpJ48DJKpHJ0).

#### Building and deploying to a test cluster

To build from source and deploy to a cluster:
  - Ensure your `kubeconfig` is configured to point at your cluster
    - On GKE, run `gcloud container clusters get-credentials <cluster-name>
      --zone <cluster-zone>`. Also ensure you run `gcloud auth configure-docker`
      so that `docker-push` works correctly.
    - To deploy to KIND, see below instead.
    - On other platforms... do whatever it is you're supposed to do (and update
      this documentation with more useful guidance!).
  - Use `make deploy` to deploy to your cluster.
    - This will also install the `kubectl-hns` plugin into `$GOPATH/bin`. Ensure
      that this is in your `PATH` env var if you want to use it by saying `kubectl
      hns`, as described in the user guide.
    - The manifests that get deployed will be output to
      `/manifests/hnc-controller.yaml` if you want to see exactly what gets
      deployed.
    - Note that `make deploy` can respond to env vars in your environment; see
      the Makefile for more information.
  - To view logs, say `make deploy-watch`

#### Special considerations for developing with KIND

While developing the HNC, it's usually faster to deploy locally to
[KIND](https://kind.sigs.k8s.io). But be aware of the following gotchas:

* When you install KIND, make sure you're _not_ in the HNC directory, otherwise
  all kinds of Go module stuff will get messed up (this is a general KIND
  issue).
* Instead of `make deploy`, say `make kind-deploy` (or, equivalently,
  `CONFIG=kind make deploy` or `make deploy CONFIG=kind`). This bypasses the
  steps where you try to push the image to a Docker registry like Docker Hub or
  GCP Container Registry (`gcr.io`).
* It's up to you to ensure that your kubectl context is pointed to your KIND
  cluster (use `kubectl config get-contexts` to see if it is).
* Run `make kind-reset` to stop any existing KIND cluster and setup a new one.
  You don't need to run this every time, only when you're first starting
  development or you think your KIND cluster is in a bad state.

In addition, KIND doesn't integrate with any identity providers - that is, you
can't add "sara@foo.com" as a "regular user." So you'll have to use service
accounts and impersonate them to test things like RBAC rules. Use `kubectl --as
system:serviceaccount:<namespace>:<sa-name>` to impersonate a service account
from the command line, [as documented
here](https://kubernetes.io/docs/reference/access-authn-authz/rbac/#referring-to-subjects).

I *think* you can also use client certificate requests but I haven't tried that
out. If you don't know what I'm talking about, you probably don't want to try it
out either. If you do, please update these docs when you get the chance!j

#### Testing changes without a test cluster

Alternatively, you can also run the controller locally (ie, not on the cluster)
by saying `make run`. You still need _a_ cluster to exist (and your kubeconfig
must be pointing to it) but it's sometimes nice to have everything running on
your machine - e.g., if you want to run a debugger.

Webhooks don't work in this mode because I haven't bothered to find an easy way
to make them work yet. We don't really use this method ourselves anymore so it
may not always work.

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
* `/test`: various end-to-end tests

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

### CI infrastructure

HNC uses Prow to run tests, which is configured
[here](https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/wg-multi-tenancy).
The presubmits run `hack/ci-test.sh` in this repo, and the postsubmits and
periodics run `hack/prow-e2e`.  Results are displayed on
[testgrid](https://k8s-testgrid.appspot.com/wg-multi-tenancy-hnc) and are
configured
[here](https://github.com/kubernetes/test-infra/tree/master/config/testgrids/kubernetes/wg-multi-tenancy).
For more information about Prow jobs (e.g. a reference for the configs), see
[here](https://github.com/kubernetes/test-infra/blob/master/prow/jobs.md).

These config files should be inspected periodically (e.g. about once a release)
to make sure they're fully up-to-date.

### Releasing

To release HNC, follow [this guide](docs/releasing.md).
