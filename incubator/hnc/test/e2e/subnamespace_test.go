package e2e

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
		MustRun("kubectl get ns", nsB)

		// The namespace "b" should have a "subnamespaceOf:a" annotation. The command to use:
		// kubectl get ns b -o jsonpath='{.metadata.annotations.hnc\.x-k8s\.io/subnamespaceOf}'
		out, err := exec.Command("kubectl", "get", "ns", nsB, "-o", "jsonpath='{.metadata.annotations.hnc\\.x-k8s\\.io/subnamespaceOf}'").Output()
		// Convert []byte to string and remove the quotes to get the "subnamesapceOf" annotation value.
		pnm := string(out)[1 : len(out)-1]
		Expect(err).Should(BeNil())
		Expect(pnm).Should(Equal(nsA))
	})
})
