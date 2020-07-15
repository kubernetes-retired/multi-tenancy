package test

import (
	"errors"
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

func mustNotRun(cmdln ...string) {
	err := tryRun(cmdln...)
	Expect(err).Should(Not(BeNil()))
}

func tryRun(cmdln ...string) error {
	stdout, err := runCommand(cmdln...)
	GinkgoT().Log("Output: ", stdout)
	return err
}

func runShouldNotContain(substr string, timeDuration string, cmdln ...string) {
	Eventually(func() error {
		stdout, err := runCommand(cmdln...)
		if err != nil {
			return err
		}
		if strings.Contains(stdout, substr) != false {
			return errors.New("Not expecting: " + substr + " but get: " + stdout)
		}
		return nil
	}, timeDuration).Should(Succeed())
}

func runCommand(cmdln ...string) (string, error) {
	var args []string
	for _, subcmdln := range cmdln {
		args = append(args, strings.Split(subcmdln, " ")...)
	}
	GinkgoT().Log("Running: ", args)
	cmd := exec.Command(args[0], args[1:]...)
	stdout, err := cmd.CombinedOutput()
	return string(stdout), err
}

func cleanupNamespaces(nses ...string) {
	// Remove all possible objections HNC might have to deleting a namesplace. Make sure it 
	// has cascading deletion so we can delete any of its subnamespace descendants, and 
	// make sure that it's not a subnamespace itself so we can delete it directly.
	for _, ns := range nses {
		tryRun("kubectl hns set", ns, "-a")
		tryRun("kubectl annotate ns", ns, "hnc.x-k8s.io/subnamespaceOf-")
		tryRun("kubectl delete ns", ns)
	}
}
