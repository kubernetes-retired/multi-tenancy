package e2e

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Namespace", func() {
	const (
		prefix = namspacePrefix+"namespace-"
		nsA = prefix+"a"
		nsB = prefix+"b"
	)

	BeforeEach(func() {
		cleanupNamespaces(nsA, nsB)
	})

	AfterEach(func() {
		cleanupNamespaces(nsA, nsB)
	})

	It("should create and delete a namespace", func() {
		// set up
		mustRun("kubectl create ns", nsA)
		mustRun("kubectl get ns", nsA)

		// test
		mustRun("kubectl", "delete", "ns", nsA)

		// verify
		mustNotRun("kubectl", "get", "ns", nsA)
	})

	It("should have 'CritParentMissing' condition on orphaned namespace", func() {
		// set up
		mustRun("kubectl create ns", nsA)
		mustRun("kubectl create ns", nsB)
		mustRun("kubectl hns set", nsB, "--parent", nsA)
		mustRun("kubectl delete ns", nsA)

		// "b" should have 'CritParentMissing' condition. The command to use:
		// kubectl get hierarchyconfigurations.hnc.x-k8s.io hierarchy -n b -o jsonpath='{.status.conditions..code}'
		out, err := exec.Command("kubectl","get","hierarchyconfigurations.hnc.x-k8s.io","hierarchy","-n",nsB,"-o", "jsonpath='{.status.conditions..code}'").Output()
		// Convert []byte to string and remove the quotes to get the condition value.
		condition := string(out)[1:len(out)-1]
		Expect(err).Should(BeNil())
		Expect(condition).Should(Equal("CritParentMissing"))
	})
})
