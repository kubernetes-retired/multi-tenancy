package test

import (
	. "github.com/onsi/ginkgo"
)

var _ = Describe("Issues", func() {

	const (
		nsParent = "issues-parent"
		nsChild = "issues-child"
	)

	BeforeEach(func() {
		cleanupNamespaces(nsParent, nsChild)
	})

	AfterEach(func() {
		cleanupNamespaces(nsParent, nsChild)
	}) 

	It("Should test issue #328: remove obsolete conditions", func() {
		// Setting up hierarchy with rolebinding that HNC doesn't have permission to copy.
		mustRun("kubectl create ns", nsParent)
		mustRun("kubectl create ns", nsChild)
		mustRun("kubectl hns set", nsChild, "--parent", nsParent)
		// cluster-admin is the highest-powered ClusterRole and HNC is missing some of
		// its permissions, so it cannot propagate it.
		mustRun("kubectl create rolebinding cluster-admin-rb -n", nsParent, 
			"--clusterrole='cluster-admin' --serviceaccount=issue-parent:default")
		// Tree should show CannotPropagateObject in nsParent and CannotUpdateObject in nsChild
		runShouldContainMultiple([]string{"1) CannotPropagateObject", "2) CannotUpdateObject"}, 1, "kubectl hns tree", nsParent)
		// Remove the child and verify that the condition is gone
		mustRun("kubectl hns set", nsChild, "--root")
		// There should no longer be any conditions in parent and child
		runShouldContain("No conditions", 1, "kubectl hns describe", nsParent)
		runShouldContain("No conditions", 1, "kubectl hns describe", nsChild)
	})
})
