package e2e

import (
	"errors"
	"testing"
	"time"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest/printer"
)

const (
	namspacePrefix = "e2e-test-"
	// The time that Eventually() will keep retrying until timeout
	// we use 5 seconds here because some tests require deleting a namespace, and shorter time period might not be enough
	eventuallyTimeout = 5
)

var hncRecoverPath = os.Getenv("HNC_REPAIR")

func TestE2e(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(time.Second * 2)
	RunSpecsWithDefaultAndCustomReporters(t,
		"HNC Suite",
		[]Reporter{printer.NewlineReporter{}})
}

func mustRun(cmdln ...string) {
	Eventually(func() error {
		return tryRun(cmdln...)
	}, eventuallyTimeout).Should(BeNil())
}

func mustNotRun(cmdln ...string) {
	Eventually(func() error {
		return tryRun(cmdln...)
	}, eventuallyTimeout).Should(Not(BeNil()))
}

func tryRun(cmdln ...string) error {
	stdout, err := runCommand(cmdln...)
	GinkgoT().Log("Output: ", stdout)
	return err
}

func tryRunSuppressLog(cmdln ...string) error {
	_, err := runCommand(cmdln...)
	return err
}

func runShouldContain(substr string, seconds float64, cmdln ...string) {
	runShouldContainMultiple([]string{substr}, seconds, cmdln...)
}

func runShouldContainMultiple(substrs []string, seconds float64, cmdln ...string) {
	Eventually(func() error {
		stdout, err := runCommand(cmdln...)
		if err != nil {
			return err
		}

		allContained := true
		for _, substr := range substrs {
			if strings.Contains(stdout, substr) == false {
				allContained = false
				break
			}
		}

		if allContained == false {
			return errors.New("Expecting: " + strings.Join(substrs, ", ") + " but get: " + stdout)
		}

		return nil
	}, seconds).Should(Succeed())
}

func runShouldNotContain(substr string, seconds float64, cmdln ...string) {
	runShouldNotContainMultiple([]string{substr}, seconds, cmdln...)
}

func runShouldNotContainMultiple(substrs []string, seconds float64, cmdln ...string) {
	Eventually(func() error {
		stdout, err := runCommand(cmdln...)
		if err != nil {
			return err
		}

		noneContained := true
		for _, substr := range substrs {
			if strings.Contains(stdout, substr) == true {
				noneContained = false
				break
			}
		}

		if noneContained == false {
			return errors.New("Not expecting: " + strings.Join(substrs, ", ") + " but get: " + stdout)
		}
		return nil
	}, seconds).Should(Succeed())
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
		tryRunSuppressLog("kubectl hns set", ns, "-a")
		tryRunSuppressLog("kubectl annotate ns", ns, "hnc.x-k8s.io/subnamespaceOf-")
		tryRunSuppressLog("kubectl delete ns", ns)
	}
}

func checkHNCPath() {
	// we don't want to destroy the HNC without being able to repair it, so skip this test if recovery path not set
	if hncRecoverPath == ""{
		Skip("Environment variable HNC_REPAIR not set. Skipping tests that require repairing HNC.")
	}
}

func recoverHNC() {
	err := tryRun("kubectl apply -f", hncRecoverPath)
	if err != nil {
		GinkgoT().Log("-----------------------------WARNING------------------------------")
		GinkgoT().Logf("WARNING: COULDN'T REPAIR HNC: %v", err)
		GinkgoT().Log("ANY TEST AFTER THIS COULD FAIL BECAUSE WE COULDN'T REPAIR HNC HERE")
		GinkgoT().Log("------------------------------------------------------------------")
		GinkgoT().FailNow()
	}
	// give HNC enough time to repair
	time.Sleep(5 * time.Second)
}
