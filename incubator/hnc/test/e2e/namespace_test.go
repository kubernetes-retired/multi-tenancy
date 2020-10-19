package e2e

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "sigs.k8s.io/multi-tenancy/incubator/hnc/pkg/testutils"
)

var _ = Describe("Namespace", func() {
	const (
		prefix = namspacePrefix + "namespace-"
		nsA    = prefix + "a"
		nsB    = prefix + "b"
	)

	BeforeEach(func() {
		CleanupNamespaces(nsA, nsB)
	})

	AfterEach(func() {
		CleanupNamespaces(nsA, nsB)
	})

	It("should create and delete a namespace", func() {
		// set up
		MustRun("kubectl create ns", nsA)
		MustRun("kubectl get ns", nsA)

		// test
		MustRun("kubectl", "delete", "ns", nsA)

		// verify
		MustNotRun("kubectl", "get", "ns", nsA)
	})

	It("should have 'ParentMissing' condition on orphaned namespace", func() {
		// set up
		MustRun("kubectl create ns", nsA)
		MustRun("kubectl create ns", nsB)
		MustRun("kubectl hns set", nsB, "--parent", nsA)
		MustRun("kubectl delete ns", nsA)

		// "b" should have 'ParentMissing' condition. The command to use:
		// kubectl get hierarchyconfigurations.hnc.x-k8s.io hierarchy -n b -o jsonpath='{.status.conditions..code}'
		out, err := exec.Command("kubectl", "get", "hierarchyconfigurations.hnc.x-k8s.io", "hierarchy", "-n", nsB, "-o", "jsonpath='{.status.conditions..reason}'").Output()
		// Convert []byte to string and remove the quotes to get the condition value.
		condition := string(out)[1 : len(out)-1]
		Expect(err).Should(BeNil())
		Expect(condition).Should(Equal("ParentMissing"))
	})
})
