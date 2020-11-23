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
		CleanupTestNamespaces()
	})

	AfterEach(func() {
		CleanupTestNamespaces()
	})

	It("should create and delete a subnamespace", func() {
		// set up
		CreateNamespace(nsA)
		MustRun("kubectl get ns", nsA)
		CreateSubnamespace(nsB, nsA)

		// verify
		FieldShouldContain("ns", "", nsB, ".metadata.annotations", "subnamespace-of:"+nsA)

		// delete
		MustRun("kubectl delete subns", nsB, "-n", nsA)
		MustNotRun("kubectl get ns", nsB)
	})
})
