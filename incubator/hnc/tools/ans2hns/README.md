# Abstract to Hierarchical Namespaces

This utility can be used to convert an Anthos Config Management or GKE Config
Sync repo to use HNC to actuate its hierarchy instead of doing it natively. It's
not an official part of HNC - it's not part of the build or release process, and
we're not releasing images for it. Please let us know if you find it useful.

## Why use this tool?

When ACM or Config Sync synchronizes a [structured
repo](https://cloud.google.com/kubernetes-engine/docs/add-on/config-sync/how-to/repo)
to a cluster, it doesn't create the [abstract namespace
directories](https://cloud.google.com/kubernetes-engine/docs/add-on/config-sync/concepts/namespace-inheritance)
on those clusters. However, in some cases, you might want those intermediate
namespaces to exist on the cluster - either because they started out as "real"
namespaces and you don't want to delete them, or because you want to put real
objects in them for some other reason (e.g. custom policies).

The `ans2hns` tool is a [KPT
function](https://googlecontainertools.github.io/kpt/guides/producer/functions/)
that allows a CS/ACM repo to be used as an [unstructured
repo](https://cloud.google.com/kubernetes-engine/docs/add-on/config-sync/how-to/unstructured-repo),
with the hierarchical features of the structure repo replaced by HNC. That is,
when run in the root of a structured repo, it does the following:

* Creates the `HierarchyConfiguration` object in each directory under
  `namespaces/` so that the resulting hierarchical namespace structure matches
  the directory structure.
  * If this object already exists in the directory, this tool simply updates it if it
    doesn't match the directory structure.
* Sets the `metadata.namespace` field of all namespaced objects to match the
  directory structure if they're missing/incorrect.

This tool does _not_ currently understand multiple repos or [namespace
selectors](https://cloud.google.com/kubernetes-engine/docs/add-on/config-sync/concepts/namespace-inheritance#excluding_namespaces_from_inheritance).
Namespace selectors must be replaced by HNC Exceptions (link to come when
published) by hand.

## Running the tool

Ensure you have [`kpt`](https://googlecontainertools.github.io/kpt/) installed
and available in your path.

To run without Docker (alpha feature in KPT):

```
# In this directory:
go install ./...
# Verify that ans2hns exists in $GOBIN, e.g. ~/go/bin

# In the root of a ConfigSync structured repo:
kpt fn run ./ --enable-exec --exec-path ans2hns
```

To run with Docker:

```
# Set up your environment (use GCR or any other registry like Docker Hub):
IMG=gcr.io/MY-PROJECT/ans2hns:latest

# In this directory:
docker build . -t ${IMG}
docker push ${IMG} # optional

# In the root of a ConfigSync structured repo:
kpt fn run ./ --image ${IMG}
```

## Updating the cluster

To switch your clusters from abstract to hierarchical namespaces after running
this tool and committing the result, you must:

* Update the ConfigSync or ACM operator to treat the repo as unstructured by
  setting the `spec.sourceFormat` field to `unstructured`.
* Install OSS HNC from the Github repo, or via Hierarchy Controller by setting
  `spec.hierarchyController` to `true` in the operator.
* Update the `HNCConfig` object to include all the object types that you want to
  propagate. By default, HC only propagates `Role` and `RoleBinding` objects,
  and all other objects will be ignored.

