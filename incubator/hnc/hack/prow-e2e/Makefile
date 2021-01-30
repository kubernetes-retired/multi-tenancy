IMG_NAME ?= gcr.io/k8s-staging-multitenancy/hnc-postsubmit-tests:latest

build:
	docker build . -t ${IMG_NAME}

# This is for local debugging. The '-it' arg is needed to allow you to hit
# ctrl-c while the tests are still running.
run: build
	@echo STARTING THE POSTSUBMIT TESTS IN THE CONTAINER
	docker run -v /var/run/docker.sock:/var/run/docker.sock --network="host" -it ${IMG_NAME}

# After calling 'make run', the Kind cluster will still be present on the
# *host* even though the Docker container has exited. Run this to find and
# delete any Kind cluster starting with 'hnc-postsubmit-'
clean:
	kind get clusters | grep hnc-postsubmit- | xargs kind delete clusters

# You need permission to push to k8s-staging-multitenancy for this to work.
push: build
	docker push ${IMG_NAME}
