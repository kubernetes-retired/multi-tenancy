package test

import (
	"testing"
	"time"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

const namspacePrefix = "e2e-test-"

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Second * 2)
	RunSpecsWithDefaultAndCustomReporters(t,
		"HNC Suite",
		[]Reporter{printer.NewlineReporter{}})
}

func mustRun(cmdln ...string) {
	err := tryRun(cmdln...)
	Expect(err).Should(BeNil())
}

func tryRun(cmdln ...string) error {
	var args []string
	for _, subcmdln := range cmdln {
		args = append(args, strings.Split(subcmdln, " ")...)
	}
	GinkgoT().Log("Running: ", args)
	cmd := exec.Command(args[0], args[1:]...)
	stdout, err := cmd.CombinedOutput()
	GinkgoT().Log("Output: ", string(stdout))
	return err
}
