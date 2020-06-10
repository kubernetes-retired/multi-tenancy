package test

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Namespace", func() {
	var (
		prefix string
	)

	BeforeEach(func() {
		prefix = namspacePrefix+"namespace-"
	})

	It("should create and delete a namespace", func() {
		// Create a namespace "a"
		cmd := exec.Command("kubectl", "create", "ns", prefix+"a")
		Expect(cmd.Run()).Should(BeNil())

		// The namespace "a" should exist.
		cmd = exec.Command("kubectl", "get", "ns", prefix+"a")
		Expect(cmd.Run()).Should(BeNil())

		// Delete the created namespace "a"
		cmd = exec.Command("kubectl", "delete", "ns", prefix+"a")
		Expect(cmd.Run()).Should((BeNil()))

		// The namespace "a" should not exist.
		cmd = exec.Command("kubectl", "get", "ns", prefix+"a")
		Expect(cmd.Run()).Should(Not(BeNil()))
	})

	It("should have 'CritParentMissing' condition on orphaned namespace", func() {
		// Create a namespace "a"
		cmd := exec.Command("kubectl", "create", "ns", prefix+"a")
		Expect(cmd.Run()).Should(BeNil())

		// Create a namespace "b"
		cmd = exec.Command("kubectl", "create", "ns", prefix+"b")
		Expect(cmd.Run()).Should(BeNil())

		// Set "b" as a child of "a"
		cmd = exec.Command("kubectl", "hns", "set", "b", "--parent", "a")
		Expect(cmd.Run()).Should(BeNil())

		// Delete the created namespace "a"
		cmd = exec.Command("kubectl", "delete", "ns", prefix+"a")
		Expect(cmd.Run()).Should((BeNil()))

		// "b" should have 'CritParentMissing' condition. The command to use:
		// kubectl get hierarchyconfigurations.hnc.x-k8s.io hierarchy -n b -o jsonpath='{.status.conditions..code}'
		out, err := exec.Command("kubectl","get","hierarchyconfigurations.hnc.x-k8s.io","hierarchy","-n","b","-o", "jsonpath='{.status.conditions..code}'").Output()
		// Convert []byte to string and remove the quotes to get the condition value.
		condition := string(out)[1:len(out)-1]
		Expect(err).Should(BeNil())
		Expect(condition).Should(Equal("CritParentMissing"))

		// Clean up - delete namespace "b"
		cmd = exec.Command("kubectl", "delete", "ns", prefix+"b")
		Expect(cmd.Run()).Should((BeNil()))
	})
})
