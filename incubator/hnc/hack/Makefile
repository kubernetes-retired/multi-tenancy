# This is a temp directory where we check out the repo.
#
# Note that this makefile does *not* attempt to clean up the temp dirs created
# above. I found some cases where I couldn't remove the files even after the
# container, images and volumes had all been nuked. So please remember to clean
# your /tmp dir from time to time, kids.
PROW_TMP := $(shell mktemp -d)

# The KRTE image to use (see prow-run-e2e.sh for information about why we use krte).
#
# The current version (20210129) includes Go 1.15.7 and kubectl 1.17. UPGRADE
# TO A NEW VERSION WHEN WE STOP SUPPORTING K8s 1.17 as that seems like a
# reasonable time. At that point, I'd just upgrade to the latest image that
# works; we don't have to increment one by one. Honestly I doubt the *exact*
# version matters too much; look at the version in
# https://github.com/kubernetes/test-infra/blob/master/config/jobs/kubernetes-sigs/kind/kind-postsubmits.yaml
# and that's probably good enough for us.
#
# Please keep this in sync with
# https://github.com/kubernetes/test-infra/tree/master/config/jobs/kubernetes-sigs/wg-multi-tenancy/hnc-e2e.yaml.
# Also, when you do this, please also upgrade the version of Kind we download
# in prow-run-e2e.sh in this directory.
PROW_IMG ?= gcr.io/k8s-testimages/krte:v20210129-3799a64-master

# This is for debugging prow-run-e2e.sh; see that script for more information.
#
# The entire repo is cloned and checked out at the master branch, except for
# prow-run-e2e.sh which is copied from this directory.
#
# The '-it' arg is needed to allow you to hit ctrl-c while the tests are still
# running.
prow-test:
	@echo ~~~ Preparing to test prow-run-e2e.sh ~~~
	@echo
	@echo Cloning the repo
	git clone https://github.com/kubernetes-sigs/multi-tenancy ${PROW_TMP}
	@echo
	@echo Copying run-e2e-tests in this directory to the synced directory
	cp -p ./prow-run-e2e.sh ${PROW_TMP}/incubator/hnc/hack
	@echo
	@echo ~~~ Testing prow-run-e2e.sh in KRTE ~~~
	@echo
	docker run \
		-e DOCKER_IN_DOCKER_ENABLED=true \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v ${PROW_TMP}:/local-test-start-dir \
		-w /local-test-start-dir \
		--network="host" \
		-it \
		${PROW_IMG} \
		/local-test-start-dir/incubator/hnc/hack/prow-run-e2e.sh

# After calling 'make prow-test', the Kind cluster might still be present on the
# *host* even though the Docker container has exited. Run this to find and
# delete any Kind cluster starting with 'hnc-postsubmit-'
prow-clean:
	kind get clusters | grep hnc-postsubmit- | xargs kind delete clusters
