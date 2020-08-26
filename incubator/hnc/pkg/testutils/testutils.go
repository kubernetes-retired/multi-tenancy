package testutils

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The time that Eventually() will keep retrying until timeout
// we use 5 seconds here because some tests require deleting a namespace, and shorter time period might not be enough
const eventuallyTimeout = 5

var hncRecoverPath = os.Getenv("HNC_REPAIR")

func FieldShouldContain(resource, ns, nm, field, want string){
	FieldShouldContainMultiple(resource, ns, nm, field, []string{want})
}

func FieldShouldContainMultiple(resource, ns, nm, field string, want []string){
	FieldShouldContainMultipleWithTimeout(resource, ns, nm, field, want, eventuallyTimeout)
}

func FieldShouldContainWithTimeout(resource, ns, nm, field, want string, timeout float64){
	FieldShouldContainMultipleWithTimeout(resource, ns, nm, field, []string{want}, timeout)
}

func FieldShouldContainMultipleWithTimeout(resource, ns, nm, field string, want []string, timeout float64){
	if ns != "" {
		RunShouldContainMultiple(want, timeout, "kubectl get", resource, nm, "-n", ns, "-o template --template={{"+field+"}}")
	} else {
		RunShouldContainMultiple(want, timeout, "kubectl get", resource, nm, "-o template --template={{"+field+"}}")
	}
}

func FieldShouldNotContain(resource, ns, nm, field, want string){
	FieldShouldNotContainMultiple(resource, ns, nm, field, []string{want})
}

func FieldShouldNotContainMultiple(resource, ns, nm, field string, want []string){
	FieldShouldNotContainMultipleWithTimeout(resource, ns, nm, field, want, eventuallyTimeout)
}

func FieldShouldNotContainWithTimeout(resource, ns, nm, field, want string, timeout float64){
	FieldShouldNotContainMultipleWithTimeout(resource, ns, nm, field, []string{want}, timeout)
}

func FieldShouldNotContainMultipleWithTimeout(resource, ns, nm, field string, want []string, timeout float64){
	if ns != "" {
		RunShouldNotContainMultiple(want, timeout, "kubectl get", resource, nm, "-n", ns, "-o template --template={{"+field+"}}")
	} else {
		RunShouldNotContainMultiple(want, timeout, "kubectl get", resource, nm, "-o template --template={{"+field+"}}")
	}
}

func MustRun(cmdln ...string) {
	MustRunWithTimeout(eventuallyTimeout, cmdln...)
}

func MustRunWithTimeout(timeout float64, cmdln ...string) {
	Eventually(func() error {
		return TryRun(cmdln...)
	}, timeout).Should(BeNil())
}

func MustNotRun(cmdln ...string) {
	Eventually(func() error {
		return TryRun(cmdln...)
	}, eventuallyTimeout).Should(Not(BeNil()))
}

func TryRun(cmdln ...string) error {
	stdout, err := RunCommand(cmdln...)
	if err != nil {
		// Add stdout to the error, since it's the error that gets displayed when a test fails and it
		// can be very hard looking at the log to see which failures are intended and which are not.
		err = fmt.Errorf("Error: %s\nOutput: %s", err, stdout)
		GinkgoT().Log("Output (failed): ", err)
	} else {
		GinkgoT().Log("Output (passed): ", stdout)
	}
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
		missing, err := runShouldContainMultiple(substrs, cmdln...)
		if err != nil {
			return err
		}
		if missing != "" {
			return errors.New(missing)
		}
		return nil
	}, seconds).Should(Succeed())
}

func RunErrorShouldContain(substr string, seconds float64, cmdln ...string) {
	RunErrorShouldContainMultiple([]string{substr}, seconds, cmdln...)
}

func RunErrorShouldContainMultiple(substrs []string, seconds float64, cmdln ...string) {
	Eventually(func() error {
		missing, err := runShouldContainMultiple(substrs, cmdln...)
		if missing != "" {
			return errors.New(missing)
		}
		if err == nil {
			return errors.New("Expecting command to fail but get succeed.")
		}
		return nil
	}, seconds).Should(Succeed())
}

func runShouldContainMultiple(substrs []string, cmdln ...string) (string, error) {
		stdout, err := RunCommand(cmdln...)
		GinkgoT().Log("Output: ", stdout)
		return missAny(substrs, stdout), err
}

// If any of the substrs are missing from teststring, returns a string of the form:
//   Missing: <string1>, <string2>, ...
//   Got: teststring
// Otherwise returns the empty string.
func missAny(substrs []string, teststring string) string {
	var missing []string
	for _, substr := range substrs {
		if strings.Contains(teststring, substr) == false {
			missing = append(missing, substr)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	// This looks *ok* if we're only missing a single multiline string, and ok if we're missing
	// multiple single-line strings. It would look awful if we were missing multiple multiline strings
	// but I think that's pretty rare.
	msg := "Missing: "+strings.Join(missing, ", ")+"\n\n"
	msg += "Got: "+teststring
	return msg
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

func MustApplyYAML(s string){
	filename := WriteTempFile(s)
	defer RemoveFile(filename)
	MustRun("kubectl apply -f", filename)
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
		if len(subcmdln) > 2 && subcmdln[0] == '"' && subcmdln[len(subcmdln)-1] == '"' {
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

// TearDownHNC removes CRDs first and then the entire manifest from the current
// change. If a specific HNC version is provided, its manifest will also be
// deleted. It will ensure HNC is cleared at the end.
func TearDownHNC(hncVersion string) {
	// Delete all CRDs first to ensure all finalizers are removed. Since we don't
	// know the version of the current HNC in the cluster so we will try tearing
	// it down twice with the specified version and what's in the HEAD.
	TryRunQuietly("k delete crd subnamespaceanchors.hnc.x-k8s.io")
	TryRunQuietly("k delete crd hierarchyconfigurations.hnc.x-k8s.io")
	TryRunQuietly("k delete crd hncconfigurations.hnc.x-k8s.io")
	TryRunQuietly("kubectl delete -f ../../manifests/hnc-manager.yaml")
	if hncVersion != ""{
		TryRunQuietly("kubectl delete -f https://github.com/kubernetes-sigs/multi-tenancy/releases/download/hnc-"+hncVersion+"/hnc-manager.yaml")
	}
	// Wait for HNC to be fully torn down (the namespace and the CRDs are gone).
	RunShouldNotContain("hnc-system", 10, "kubectl get ns")
	RunShouldNotContain(".hnc.x-k8s.io", 10, "kubectl get crd")
}

func CheckHNCPath() {
	// we don't want to destroy the HNC without being able to repair it, so skip this test if recovery path not set
	if hncRecoverPath == "" {
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

func WriteTempFile(cxt string) string {
	f, err := ioutil.TempFile(os.TempDir(), "e2e-test-*.yaml")
	Expect(err).Should(BeNil())
	defer f.Close()
	f.WriteString(cxt)
	return f.Name()
}

func RemoveFile(path string) {
	Expect(os.Remove(path)).Should(BeNil())
}
