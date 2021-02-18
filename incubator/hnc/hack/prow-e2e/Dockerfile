# krte includes everything needed to run Docker-in-Docker, as well as cleaning
# it up cleanly. Note that Prow uses containerd so typically, no Docker daemon
# will be running on the node without using the contents of this image!
#
# This includes Go 1.15.7 and kubectl 1.17. UPGRADE TO A NEW VERSION WHEN WE
# STOP SUPPORTING K8s 1.17 as that seems like a reasonable time. At that point,
# I'd just upgrade to the latest image that works; we don't have to increment
# one by one. Honestly I doubt the exact version of the base image makes too
# much of a difference.
#
# Please update the version of Kind at the same time you update the version of
# KRTE, if applicable!
#
# Source: https://github.com/kubernetes/test-infra/tree/master/images/krte
FROM gcr.io/k8s-testimages/krte:v20210129-3799a64-master

# For postsubmits, Prow will start the test in its own workdir (e.g.
# /home/prow/go/src/sigs.k8s.io/multi-tenancy, though we shouldn't depend on
# that). For periodics, I don't think Prow overrides the workdir. Either way,
# set up the workdir in a place that makes sense for the periodics, but also
# allows us to easily find the test script even if it's changed.
WORKDIR /start

# I'm not sure if the following two env vars are needed for HNC; they're probably not. GO111MODULE=on
# should be redundant outside of GOPATH (which we are) on any post-1.13 version of Go (which we have);
# I don't *think* that we need CGO either. Feel free to try out removing them when you're next playing
# around with this image.
ENV GO111MODULE=on
ENV CGO_ENABLED=0

# Install Kind. Feel free to update this version whenever you're updating the
# KRTE base image too.
RUN go get sigs.k8s.io/kind@v0.9.0

# Add the actual script and set it as the default command for this image.
COPY run-e2e-tests.sh run-e2e-tests.sh
RUN chmod +x ./run-e2e-tests.sh
