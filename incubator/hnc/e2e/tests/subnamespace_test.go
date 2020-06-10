package test

import (
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Subnamespaces", func() {
	var (
		prefix string
	)

	BeforeEach(func() {
		prefix = namspacePrefix+"subnamespace-"
	})

	It("should create and delete a subnamespace", func() {
		// Create a namespace "a"
		cmd := exec.Command("kubectl", "create", "ns", prefix+"a")
		Expect(cmd.Run()).Should(BeNil())

		// The namespace "a" should exist.
		cmd = exec.Command("kubectl", "get", "ns", prefix+"a")
		Expect(cmd.Run()).Should(BeNil())

		// Create subnamespace "b" for "a"
		cmd = exec.Command("kubectl", "hns", "create", prefix+"b", "-n", prefix+"a")
		Expect(cmd.Run()).Should(BeNil())

		// The namespace "b" should exist.
		cmd = exec.Command("kubectl", "get", "ns", prefix+"b")
		Expect(cmd.Run()).Should((BeNil()))

		// The namespace "b" should have a "subnamespaceOf:a" annotation. The command to use:
		// kubectl get ns b -o jsonpath='{.metadata.annotations.hnc\.x-k8s\.io/subnamespaceOf}'
		out, err := exec.Command("kubectl", "get", "ns", prefix+"b", "-o", "jsonpath='{.metadata.annotations.hnc\\.x-k8s\\.io/subnamespaceOf}'").Output()
		// Convert []byte to string and remove the quotes to get the "subnamesapceOf" annotation value.
		pnm := string(out)[1:len(out)-1]
		Expect(err).Should(BeNil())
		Expect(pnm).Should(Equal(prefix+"a"))

		// Clean up by setting allowCascadingDelete on "a" and then deleting "a".
		cmd = exec.Command("kubectl", "hns", "set", prefix+"a", "-a")
		Expect(cmd.Run()).Should((BeNil()))
		cmd = exec.Command("kubectl", "delete", "ns", prefix+"a")
		Expect(cmd.Run()).Should((BeNil()))
	})
})
