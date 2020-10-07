package e2e

import (
	. "github.com/onsi/ginkgo"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("Subnamespaces", func() {
	const (
		prefix = namspacePrefix + "subnamespace-"
		nsA    = prefix + "a"
		nsB    = prefix + "b"
	)

	BeforeEach(func() {
		CleanupNamespaces(nsA, nsB)
	})

	AfterEach(func() {
		CleanupNamespaces(nsA, nsB)
	})

	It("should create and delete a subnamespace", func() {
		// set up
		MustRun("kubectl create ns", nsA)
		MustRun("kubectl get ns", nsA)
		MustRun("kubectl hns create", nsB, "-n", nsA)

		// verify
		FieldShouldContain("ns", "", nsB, ".metadata.annotations", "subnamespace-of:"+nsA)

		// delete
		MustRun("kubectl delete subns", nsB, "-n", nsA)
		MustNotRun("kubectl get ns", nsB)
	})
})
