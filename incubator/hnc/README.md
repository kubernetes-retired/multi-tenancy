# The Hierarchical Namespace Controller (HNC)

***[UPDATE MAY 2021] HNC has moved to its own repo:
https://github.com/kubernetes-sigs/hierarchical-namespaces. This version of HNC
is no longer being maintained; please visit the new repo for all future
development.***

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

The [latest version of HNC available **from this repo** is
v0.8.0](https://github.com/kubernetes-sigs/multi-tenancy/releases/tag/hnc-v0.8.0).
***[UPDATED MAY 2021] Newer versions may be available at the [new
repo](https://github.com/kubernetes-sigs/hierarchical-namespaces)***.

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

* ***Post v0.8: please visit the [new
  repo](https://github.com/kubernetes-sigs/hierarchical-namespaces)***
* [v0.8 - COMPLETE APR 2021](https://github.com/kubernetes-sigs/multi-tenancy/milestone/20):
  incremental stability improvements
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

***HNC has moved to its own repo:
https://github.com/kubernetes-sigs/hierarchical-namespaces. This version of HNC
is no longer being maintained as of May 2021; please visit the new repo for all
future development.***

