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
the release branch._ The user guide should usually contain instructions for at
least the last two minor releases of HNC - e.g., if the current version is v0.5,
it should contain instructions for both v0.4 and v0.5. But if you're _about_ to
release v0.6, then you should:

* In the master docs README, add a link to the v0.4 version of the user guide.
* Delete everything about v0.4
* Add all new information for v0.6.

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
2. Start by copying the text from earlier releases - e.g., include installation
   instructions, key new features, a detailed change log, known issues, and a
   test signoff grid. Modify it as appropriate for your new release. The test
   signoff grid will start out empty.
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

Fill in the test signoff grid in the release notes. We generally test on the
three GKE channels (rapid, regular and stable) as well as the latest KIND
version.

To test GKE, ensure you've (manually) downloaded the kubectl plugin according to
your release instructions, set your kubectl context to point to the cluster
you're testing, install HNC, then run:

```bash
export HNC_REPAIR=<path to installable YAML; https is fine>
make test-e2e
```

This will take up to 30m per cluster. As each cluster passes, update the test
grid.

If you've previously tested a release candidate and then have built a new
"final" image with no changes, you don't have to fully re-test - just copy the
signoff grid from the RC notes, modify their results to say "(as RC1)" (for
example), and then rerun the e2e tests on _one_ cluster. That should be enough
coverage.

## Update docs

Go back to your release and edit the description to remove the "do not use"
warning. If this was a release candidate, you're done.

Otherwise, update the [README](../README.md#start) and [user
guide](user-guide/how-to.md#admin-install) to refer to your new release.

If this was a patch release _and you need to document something_, ensure you
document it on _both_ the master _and_ the release branch.

## Update Krew

Starting with HNC v0.6.x, the build process also generates a Krew tarball and
manifest. This manifest should be downloaded and checked into the Krew index,
*if* it's for the latest branch (and is not a release candidate). E.g. if you've
already released HNC v0.7.0 and have to release HNC v0.6.1, do *not* update the
Krew index; Krew can only support one version of a plugin at a time so we should
only support the most recent branch.

Once the Krew manifest has been generated, [test it
locally](https://krew.sigs.k8s.io/docs/developer-guide/testing-locally/):
download the manifest and try to install it via `kubectl krew install
--manifest=FILENAME` (do _not_ override the archive as you want to use the
released archive). Once it's working, create a PR to
https://github.com/kubernetes-sigs/krew-index, such as [this
one](https://github.com/kubernetes-sigs/krew-index/pull/890).

If you like, you can strike out the Krew installation instructions on the
release notes until this PR is approved (include a link to the PR in the release
notes page until that's done). You don't have to do this if the _last_ version
of the Krew plugin will still work with the newest version of HNC.

## Mark the release as ready

In the Github UI, go back to the release notes and remove the "do not use"
warning message. If this is not an RC, remove the "this is prerelease" checkmark
so that this becomes the default release shown to users.

If you're still waiting for the Krew PR to be approved, remember to go back and
update the release notes once that's done.

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
