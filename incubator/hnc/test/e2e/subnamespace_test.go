package test

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subnamespaces", func() {
	const (
		prefix = namspacePrefix+"subnamespace-"
		nsA = prefix+"a"
		nsB = prefix+"b"
	)

	BeforeEach(func() {
		cleanupNamespaces(nsA, nsB)
	})

	AfterEach(func() {
		cleanupNamespaces(nsA, nsB)
	})

	It("should create and delete a subnamespace", func() {
		// set up
		mustRun("kubectl create ns", nsA)
		mustRun("kubectl get ns", nsA)
		mustRun("kubectl hns create", nsB, "-n", nsA)

		// verify
		mustRun("kubectl get ns", nsB)

		// The namespace "b" should have a "subnamespaceOf:a" annotation. The command to use:
		// kubectl get ns b -o jsonpath='{.metadata.annotations.hnc\.x-k8s\.io/subnamespaceOf}'
		out, err := exec.Command("kubectl", "get", "ns", nsB, "-o", "jsonpath='{.metadata.annotations.hnc\\.x-k8s\\.io/subnamespaceOf}'").Output()
		// Convert []byte to string and remove the quotes to get the "subnamesapceOf" annotation value.
		pnm := string(out)[1:len(out)-1]
		Expect(err).Should(BeNil())
		Expect(pnm).Should(Equal(nsA))
	})
})
