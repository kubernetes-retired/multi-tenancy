# This is for local debugging. The entire repo is cloned and checked out at the
# master branch, except for run-e2e-tests.sh which is copied from this
# directory.
#
# The '-it' arg is needed to allow you to hit ctrl-c while the tests are still
# running.
TMPDIR := $(shell mktemp -d)
IMG_NAME ?= gcr.io/k8s-testimages/krte:v20210129-3799a64-master
test:
	@echo
	@echo Cloning the repo
	git clone https://github.com/kubernetes-sigs/multi-tenancy ${TMPDIR}
	@echo
	@echo Copying run-e2e-tests in this directory to the synced directory
	cp ./run-e2e-tests.sh ${TMPDIR}/incubator/hnc/hack/prow-e2e
	chmod a+x ${TMPDIR}/incubator/hnc/hack/prow-e2e/run-e2e-tests.sh
	@echo
	@echo STARTING THE POSTSUBMIT TESTS IN THE CONTAINER
	docker run \
		-e DOCKER_IN_DOCKER_ENABLED=true \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v ${TMPDIR}:/local-test-start-dir \
		-w /local-test-start-dir \
		--network="host" \
		-it \
		${IMG_NAME} \
		/local-test-start-dir/incubator/hnc/hack/prow-e2e/run-e2e-tests.sh

# After calling 'make run', the Kind cluster will still be present on the
# *host* even though the Docker container has exited. Run this to find and
# delete any Kind cluster starting with 'hnc-postsubmit-'
clean:
	kind get clusters | grep hnc-postsubmit- | xargs kind delete clusters
