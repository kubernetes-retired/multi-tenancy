package testutils

import (
	"errors"
	"time"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The time that Eventually() will keep retrying until timeout
// we use 5 seconds here because some tests require deleting a namespace, and shorter time period might not be enough
const eventuallyTimeout = 5

var hncRecoverPath = os.Getenv("HNC_REPAIR")

func MustRun(cmdln ...string) {
	Eventually(func() error {
		return TryRun(cmdln...)
	}, eventuallyTimeout).Should(BeNil())
}

func MustNotRun(cmdln ...string) {
	Eventually(func() error {
		return TryRun(cmdln...)
	}, eventuallyTimeout).Should(Not(BeNil()))
}

func TryRun(cmdln ...string) error {
	stdout, err := RunCommand(cmdln...)
	GinkgoT().Log("Output: ", stdout)
	return err
}

func TryRunQuietly(cmdln ...string) error {
	_, err := RunCommand(cmdln...)
	return err
}

func RunShouldContain(substr string, seconds float64, cmdln ...string) {
	RunShouldContainMultiple([]string{substr}, seconds, cmdln...)
}

func RunShouldContainMultiple(substrs []string, seconds float64, cmdln ...string) {
	Eventually(func() error {
		stdout, err := RunCommand(cmdln...)
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

func RunShouldNotContain(substr string, seconds float64, cmdln ...string) {
	RunShouldNotContainMultiple([]string{substr}, seconds, cmdln...)
}

func RunShouldNotContainMultiple(substrs []string, seconds float64, cmdln ...string) {
	Eventually(func() error {
		stdout, err := RunCommand(cmdln...)
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

// RunCommand passes all arguments to the OS to execute, and returns the combined stdout/stderr and
// and error object. By default, each arg to this function may contain strings (e.g. "echo hello
// world"), in which case we split the strings on the spaces (so this would be equivalent to calling
// "echo", "hello", "world"). If you _actually_ need an OS argument with strings in it, pass it as
// an argument to this function surrounded by double quotes (e.g. "echo", "\"hello world\"" will be
// passed to the OS as two args, not three).
func RunCommand(cmdln ...string) (string, error) {
	var args []string
	for _, subcmdln := range cmdln {
		// Any arg that starts and ends in a double quote shouldn't be split further
		if len(subcmdln)>2 && subcmdln[0]=='"' && subcmdln[len(subcmdln)-1]=='"' {
			args = append(args, subcmdln[1:len(subcmdln)-1])
		} else {
			args = append(args, strings.Split(subcmdln, " ")...)
		}
	}
	GinkgoT().Log("Running: ", args)
	cmd := exec.Command(args[0], args[1:]...)
	stdout, err := cmd.CombinedOutput()
	return string(stdout), err
}

func CleanupNamespaces(nses ...string) {
	// Remove all possible objections HNC might have to deleting a namesplace. Make sure it 
	// has cascading deletion so we can delete any of its subnamespace descendants, and 
	// make sure that it's not a subnamespace itself so we can delete it directly.
	for _, ns := range nses {
		TryRunQuietly("kubectl hns set", ns, "-a")
		TryRunQuietly("kubectl annotate ns", ns, "hnc.x-k8s.io/subnamespaceOf-")
		TryRunQuietly("kubectl delete ns", ns)
	}
}

func CheckHNCPath() {
	// we don't want to destroy the HNC without being able to repair it, so skip this test if recovery path not set
	if hncRecoverPath == ""{
		Skip("Environment variable HNC_REPAIR not set. Skipping tests that require repairing HNC.")
	}
}

func RecoverHNC() {
	err := TryRun("kubectl apply -f", hncRecoverPath)
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
