# Releasing HNC

HNC uses semantic versioning. We use long-lived branches for every minor
release, and each release is tagged in Git. Usually, a release will be preceeded
by one or more release candidates. Therefore, at a high level, the flow to
release a new _minor_ version of HNC is:

1. Create a release branch named `hnc-vMAJOR.MINOR`, such as `hnc-v0.5`. Note
   that while regular semantic versioning is just `vMAJOR.MINOR`, we need to use
   the `hnc-` prefix because this repo contains projects other than HNC.
2. Use the Github UI to tag the first commit on that branch as
   `hnc-vMAJOR.MINOR.0-rc1`, such as `hnc-v0.5.0-rc1`. Release and test
   according to the instructions below.
3. If more release candidates are needed, number them sequentially. When you're
   happy with it, create a new release without the `-rcX` suffix, like
   `hnc-v0.5.0`.

To release a _patch_ version of HNC (e.g. `hnc-v0.5.1`), follow the same steps
but without creating a branch. Note that patches can have release candidates
just like minor releases.

## Prerequisites

You must have permission to write to this repo, and create a [personal access
token](https://docs.github.com/en/github/authenticating-to-github/creating-a-personal-access-token)
that includes that permission.

You also need the ability to push to `gcr.io/k8s-staging-multitenancy`. You can
get this by joining the k8s-infra-staging-multitenancy@kubernetes.io Google
Group, which also gives you access to the `k8s-staging-multitenancy` GCP project
(this is a standalone project and isn't in a GCP Organization).

Finally, you must have a GCP project with Cloud Build enabled, and `gcloud` must
be configured with this as your default project. _TODO: create a central project
that anyone can use, but without leaking personal access tokens._

## Document new/changed features

Ensure that the [user guide](user-guide/) is up-to-date with all the latest or
changed features. _This must be done on the master branch **before** creating
the release branch._ The user guide should contain instructions for at least the
last two minor releases of HNC - e.g., if you're about to release v0.6, do not
remove documentation for HNC v0.4 yet.

## Create a release branch

Assuming that the Github repo is your `upstream` remote, simply do the following
steps:

```bash
# Make sure you don't have any modified files in your repo.
$ git checkout master
$ git pull
$ git checkout -b <branch name>
$ git push upstream <branch name>
```

Do _not_ create a pull request for this branch! Instead, if you're a repo
administrator, go to the Github UI and mark the branch as protected, which will
prevent people from deleting it by accident. Otherwise, ask a repo admin to do
this for you.

## Create a release

### Set up your environment

Set the following environment variables:

```bash
export MT_ENDPOINT=https://api.github.com/repos/kubernetes-sigs/multi-tenancy
export HNC_USER=<your github name>
export HNC_PAT=<your personal access token>
export HNC_IMG_TAG=<the semantic version, eg v0.1.0-rc1>
```

Note that `HNC_IMG_TAG` does _not_ include the `hnc-` prefix. That is because
the image tag will only apply to HNC images, while the _Git_ tag (and branch)
names apply to this repo, which includes non-HNC projects.

### Create a release in Github

1. Ensure that the Github tag name is `hnc-$HNC_IMG_TAG`, like `hnc-v0.1.0-rc1`.
2. Follow the pattern in earlier releases in the description - e.g. include
   installation instructions, key new features, a detailed change log, and known
   issues.
3. Add a "**RELEASE IN PROGRESS, DO NOT USE**" warning to the top of the
   description.
4. If this is a release candidate, mark the release as pre-production.
5. Save the release. This will create a tag in the git repo.

### Get the release ID

Get the release ID. Sadly, this isn't available through the UI, so get it by
calling:

```bash
curl -u "$HNC_USER:$HNC_PAT" $MT_ENDPOINT/releases | less
```

Finding your release, and noting it's ID. Save this as an env var:

`export HNC_RELEASE_ID=<id>`

### Build the image and manifests

From your local repo, call `make release`. This will use `gcloud` to submit a
job to Cloud Build. Note that your local files are _not_ used to build HNC;
Cloud Build will pull them directly from Github using your personal access
token.

**Note that your personal access token will be visible in the build logs,** but
will not be printed to the console from `make` itself. TODO: [fix
this](https://cloud.google.com/cloud-build/docs/securing-builds/use-encrypted-secrets-credentials#example_build_request_using_an_encrypted_variable).

## Test

Test! At a minimum, install it onto a cluster, download the kubectl plugin, and
run:

```bash
export HNC_REPAIR=<path to installable YAML; https is fine>
make e2e-test
```

This will take around 15m or so. It might also be prudent to run through the
demo (soon to be part of the e2e test as well).

## Update docs

Go back to your release and edit the description to remove the "do not use"
warning. If this was a release candidate, you're done.

Otherwise, update the [README](../README.md#start) and [user
guide](user-guide/how-to.md#admin-install) to refer to your new release.

Finally, if this is a minor (not patch) release, remove any obsolete
documentation from the guide - e.g., if you've just released 0.6, you can remove
references to 0.4. Add a link in the [front page](user-guide/README.md) to the
guide on the branch that you've just removed.

On the other hand, if this was a patch release _and you need to document
something_, ensure you document it on _both_ the master _and_ the release
branch.

We may revise this guidance as HNC matures.

## Track usage

After the release, you can run the same command you used to find the release ID
to see how many times each asset has been downloaded.

## Updating and testing the release process

You can test the release process by pushing everything to your own Github repo,
creating a release, tag etc in *your* repo, and then setting the following env
vars:

* `HNC_RELEASE_REPO_OWNER`: this is the Github repo owner - default is
  `kubernetes-sigs`, replace with your name (e.g. `adrianludwin`). The
  `multi-tenancy` repo name is hardcoded and can't be changed.
* `HNC_RELEASE_REGISTRY`: default is `gcr.io/k8s-staging-multitenancy`, replace
  with your own registry (eg `gcr.io/adrians-project`).

This will build from the specified repo and release the image to the specified
registry.
